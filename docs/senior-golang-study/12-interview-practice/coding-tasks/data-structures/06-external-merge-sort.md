# Задача 6: External Merge Sort (сортировка, не влезающая в память)

Классическая «логическая» задача про большие данные: датасет на порядки больше RAM, нужно отсортировать «через диск». Проверяет понимание иерархии памяти, I/O и того, что делать, когда `O(N) памяти` недоступно.

## Формулировка

> «Есть сервер: многоядерный CPU, **128 MB** оперативной памяти, **16 TB** HDD. На диске лежит файл **1 TB**, построчно хранит числа `int64`. Отсортируй его.»

Суть: данные (1 TB) **в ~8000 раз** больше RAM (128 MB) → целиком в память не поднять. Сортируем **внешней сортировкой слиянием** (external merge sort): диск выступает рабочим пространством.

---

## Уточняющие вопросы (signal seniority — спросить ДО кода)

1. **Сколько RAM реально доступно под данные?**
   "Из 128 MB резерв под ОС, рантайм, I/O-буферы — реально ~100 MB под полезные данные."

2. **Формат входа и выхода — текст или бинарь?**
   "Вход текстовый (числа построчно). Временные файлы лучше делать **бинарными** (8 байт на int64): меньше места, нет парсинга на каждом проходе."

3. **Хватает ли места на диске под временные файлы?**
   "16 TB при входе 1 TB — с запасом. Внешней сортировке нужно ~2× датасета на scratch."

4. **Нужна ли стабильность / уникальность?**
   "Числа — порядок равных не важен (стабильность не нужна). Дубликаты и отрицательные `int64` — штатно."

5. **Можно ли портить исходный файл / какой вид у результата?**
   "Результат — отдельный отсортированный файл; вход не трогаем."

6. **Использовать многоядерность?**
   "Да: генерацию ранов и часть слияния можно распараллелить. Но узкое место — полоса HDD, не CPU."

---

## Идея: external merge sort в две фазы

```
ФАЗА 1 — нарезка + сортировка (run generation)
1 TB вход ──читаем кусками ~100 MB──► sort в RAM ──► sorted run на HDD (бинарь)
                                                       run_0, run_1, ... run_M  (тысячи)

ФАЗА 2 — k-путёвое слияние (k-way merge)
run_0 ─┐
run_1 ─┤  min-heap по голове каждого рана ──► пишем минимум ──► отсортированный выход
 ...   │  (за один проход сливаем ~k ранов: памяти хватает только на k буферов)
run_k ─┘
многопроходно, пока ранов > k  ⇒  число проходов = log_k(число_ранов) ≈ 2
```

- **Фаза 1** превращает 1 TB хаоса в тысячи **отсортированных** кусков, каждый из которых помещался в RAM.
- **Фаза 2** сливает их, читая из каждого рана по чуть-чуть (буфер) и выбирая глобальный минимум через кучу. Поскольку под буферы всех ранов сразу памяти нет — сливаем **группами** в несколько проходов.

---

## Базовое решение

### Фаза 1 — генерация отсортированных ранов

```go
package extsort

import (
	"bufio"
	"encoding/binary"
	"os"
	"slices"
	"strconv"
)

// chunkInts — сколько int64 держим в памяти за раз (~ доступная RAM / 8 байт).
func generateRuns(inputPath, tmpDir string, chunkInts int) ([]string, error) {
	in, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20) // буфер строки до 1 MB

	buf := make([]int64, 0, chunkInts)
	var runs []string

	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		slices.Sort(buf) // in-memory сортировка куска
		name, err := writeRun(tmpDir, buf)
		if err != nil {
			return err
		}
		runs = append(runs, name)
		buf = buf[:0]
		return nil
	}

	for sc.Scan() {
		n, err := strconv.ParseInt(sc.Text(), 10, 64)
		if err != nil {
			return nil, err
		}
		buf = append(buf, n)
		if len(buf) == cap(buf) { // кусок заполнил RAM — сбрасываем
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil { // хвост
		return nil, err
	}
	return runs, nil
}

// writeRun пишет отсортированный кусок бинарно (8 байт/число, little-endian).
func writeRun(tmpDir string, data []int64) (string, error) {
	f, err := os.CreateTemp(tmpDir, "run-*.bin")
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 1<<20)
	var b [8]byte
	for _, v := range data {
		binary.LittleEndian.PutUint64(b[:], uint64(v)) // знаковый int64 ↔ uint64 битово
		if _, err := w.Write(b[:]); err != nil {
			return "", err
		}
	}
	if err := w.Flush(); err != nil {
		return "", err
	}
	return f.Name(), nil
}
```

### Фаза 2 — k-way merge через `container/heap`

```go
import "container/heap"

type runReader struct {
	r   *bufio.Reader
	f   *os.File
	src int
}

func (rr *runReader) next() (int64, bool) {
	var b [8]byte
	if _, err := io.ReadFull(rr.r, b[:]); err != nil {
		return 0, false // EOF / ошибка → ран исчерпан
	}
	return int64(binary.LittleEndian.Uint64(b[:])), true
}

// min-heap: храним текущую "голову" каждого рана + индекс источника.
type hItem struct {
	val int64
	src int
}
type minHeap []hItem

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i].val < h[j].val }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)         { *h = append(*h, x.(hItem)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

// mergeRuns сливает <=k ранов в один отсортированный выходной файл.
func mergeRuns(runs []string, outPath string) error {
	readers := make([]*runReader, 0, len(runs))
	for i, name := range runs {
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		readers = append(readers, &runReader{r: bufio.NewReaderSize(f, 1<<20), f: f, src: i})
	}
	defer func() {
		for _, rr := range readers {
			rr.f.Close()
		}
	}()

	h := &minHeap{}
	for _, rr := range readers { // инициализируем кучу первыми элементами
		if v, ok := rr.next(); ok {
			heap.Push(h, hItem{val: v, src: rr.src})
		}
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	w := bufio.NewWriterSize(out, 1<<20)
	defer w.Flush()

	var b [8]byte
	for h.Len() > 0 {
		it := heap.Pop(h).(hItem)
		binary.LittleEndian.PutUint64(b[:], uint64(it.val))
		if _, err := w.Write(b[:]); err != nil {
			return err
		}
		// подтянуть следующий элемент из того же рана
		if v, ok := readers[it.src].next(); ok {
			heap.Push(h, hItem{val: v, src: it.src})
		}
	}
	return nil
}
```

---

## Production-grade: многопроходное слияние + параллелизм

Тысячи ранов нельзя слить за один проход — под буфер каждого рана нужна память. При RAM 128 MB и буфере ~1 MB (чтобы чтение с HDD было последовательным) `k ≈ 100` ранов за проход. Решение — **многопроходное слияние группами**:

```go
// externalSort: полный пайплайн. maxFanIn = сколько ранов сливаем за проход (~RAM/буфер).
func externalSort(inputPath, outputPath, tmpDir string, chunkInts, maxFanIn int) error {
	runs, err := generateRuns(inputPath, tmpDir, chunkInts) // Фаза 1 (параллелится)
	if err != nil {
		return err
	}

	pass := 0
	for len(runs) > 1 {
		var next []string
		// бьём раны на группы по maxFanIn и сливаем каждую в новый ран
		for i := 0; i < len(runs); i += maxFanIn {
			group := runs[i:min(i+maxFanIn, len(runs))]
			merged, err := os.CreateTemp(tmpDir, "merge-*.bin")
			if err != nil {
				return err
			}
			merged.Close()
			if err := mergeRuns(group, merged.Name()); err != nil {
				return err
			}
			next = append(next, merged.Name())
			for _, r := range group { // подчищаем израсходованные раны — диск не бесконечен
				os.Remove(r)
			}
		}
		runs = next
		pass++
	}
	// runs[0] — отсортированный бинарь; при необходимости конвертируем обратно в текст
	return binToText(runs[0], outputPath)
}
```

Оптимизации, которые стоит назвать:

| Приём | Зачем |
|---|---|
| **Бинарные временные файлы** (8 байт/число) | меньше I/O и места, нет парсинга строк на каждом проходе |
| **Большие последовательные буферы** (≥1 MB) | HDD быстр на sequential, медленный на random — избегаем seek'ов |
| **Многоядерная Фаза 1** | сортировку независимых кусков раздаём горутинам/воркер-пулу |
| **Параллельные группы слияния** | разные группы ранов сливаются независимо на разных ядрах |
| **Replacement selection** | раны через heap дают в среднем ×2 длину → меньше ранов → меньше проходов |
| **Radix/LSD-сортировка куска** | ключи фиксированной длины 8 байт → O(N) вместо O(N log N) |

---

## Тесты

```go
func TestExternalSort(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.txt")

	// генерируем вход с дублями и отрицательными
	want := []int64{}
	var sb strings.Builder
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 100_000; i++ {
		v := int64(r.Intn(1_000_000) - 500_000)
		want = append(want, v)
		sb.WriteString(strconv.FormatInt(v, 10))
		sb.WriteByte('\n')
	}
	os.WriteFile(in, []byte(sb.String()), 0o644)
	slices.Sort(want)

	out := filepath.Join(tmp, "out.txt")
	// маленькие chunk/fanIn, чтобы форсировать МНОГО ранов и НЕСКОЛЬКО проходов
	if err := externalSort(in, out, tmp, 1000, 4); err != nil {
		t.Fatal(err)
	}

	got := readInts(t, out)
	if !slices.Equal(got, want) {
		t.Fatalf("not sorted correctly: len got=%d want=%d", len(got), len(want))
	}
}
```

Что проверять: маленькие `chunkInts`/`maxFanIn` форсируют тысячи ранов и многопроходное слияние на крошечном датасете — так ловятся баги, не видные при «всё влезло в один ран».

---

## Подводные камни

- **Пытаться отсортировать всё в памяти** — главный провал. Сразу проговорить «данные >> RAM → external sort».
- **Один проход слияния тысяч ранов** — не хватит памяти на буферы; нужно многопроходное слияние группами.
- **Маленькие/random I/O** — построчное чтение текста без буфера или мелкие записи убивают HDD на seek'ах. Только большие последовательные буферы.
- **Текстовые временные файлы** — парсинг `int64` на каждом из нескольких проходов дорог; храните раны бинарно.
- **Не удалять израсходованные раны** — на каждом проходе создаются новые файлы; без чистки можно упереться в диск.
- **`int64` ↔ `uint64`** — для бинарной записи берём биты (`uint64(v)`), но при сравнении используем **знаковый** `int64`, иначе отрицательные «уедут» в конец.
- **Переполнение буфера сканера** — у `bufio.Scanner` дефолтный лимит строки 64 KB; для длинных строк поднять `Buffer`.

---

## Оценка стоимости

- Проходов по данным: `1 (нарезка) + log_k(ранов) (слияние) ≈ 3`.
- Каждый проход = чтение + запись всего датасета. На HDD ~150 MB/s один проход по ~680 GB бинаря ≈ час+ → **несколько часов суммарно**.
- Узкое место — **полоса диска**, а не CPU; поэтому минимизируем число проходов (крупнее раны, больше fan-in) и держим I/O последовательным.

---

## Возможные расширения

- **Распределённая сорт␫ировка** — данные на нескольких машинах (MapReduce/Spark: partition по диапазону → локальный sort → конкатенация). Тот же принцип, что и k-way merge, но по сети.
- **SSD вместо HDD** — random I/O дешевеет, можно агрессивнее по fan-in и меньше думать о sequential.
- **Сортировка строк/записей переменной длины** — раны хранят offset’ы; усложняется формат, но схема та же.
- **Top-N вместо полной сортировки** — если нужны не все, а N наибольших, хватит heap размера N за один проход (см. родственную задачу).

---

## Связки с другими темами

- [Top-K Elements](./02-top-k.md) — тот же мотив «данные не влезают в память», min-heap, O(K) памяти.
- [Sorting and heap](../../../16-algorithms-and-data-structures/07-sorting-and-heap.md) — теория merge sort и кучи.
- [Stack and heap / иерархия памяти](../../../01-go-core/memory-internals/01-stack-and-heap.md) — почему RAM ограничена и при чём тут диск.
- [Memory hierarchy](../../../10-devops-and-observability/hardware-and-os/02-memory-hierarchy.md) — RAM vs диск, цена random I/O.
- [Streaming aggregation](../streams/03-streaming-aggregation.md) — обработка потоков с ограниченной памятью.
