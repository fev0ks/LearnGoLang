# Задача 6: External Merge Sort (сортировка, не влезающая в память)

## Содержание

- [Формулировка](#формулировка)
- [Уточняющие вопросы](#уточняющие-вопросы-signal-seniority--спросить-до-кода)
- [Две фазы external merge sort](#идея-external-merge-sort-в-две-фазы)
- [Генерация ранов](#фаза-1--генерация-отсортированных-ранов)
- [K-way merge](#фаза-2--k-way-merge-через-containerheap)
- [Многопроходное слияние](#многопроходное-слияние-и-параллелизм)
- [Тесты](#тесты)
- [Типичные ошибки](#подводные-камни)
- [Оценка стоимости](#оценка-стоимости)
- [Interview-ready answer](#interview-ready-answer)
- [Связанные материалы](#связки-с-другими-темами)

External merge sort сортирует данные, которые не помещаются в оперативную
память. Память ограничивает размер одновременно сортируемого куска и число
входных буферов при слиянии; диск хранит промежуточные отсортированные раны.

---

## Формулировка

> «Есть сервер: многоядерный CPU, **128 MB** оперативной памяти, **16 TB** HDD. На диске лежит файл **1 TB**, построчно хранит числа `int64`. Отсортируй его.»

При одинаковом выборе десятичных или двоичных единиц `1 TB / 128 MB` даёт
порядок восьми тысяч. Но число записей и размер бинарных временных файлов нельзя
получить только из размера текстового входа: они зависят от средней длины строки.

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
    "container/heap"
    "encoding/binary"
    "errors"
    "io"
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

    w := bufio.NewWriterSize(f, 1<<20)
    var b [8]byte
    for _, v := range data {
        binary.LittleEndian.PutUint64(b[:], uint64(v)) // знаковый int64 ↔ uint64 битово
        if _, err := w.Write(b[:]); err != nil {
            f.Close()
            return "", err
        }
    }
    if err := w.Flush(); err != nil {
        f.Close()
        return "", err
    }
    if err := f.Close(); err != nil {
        return "", err
    }
    return f.Name(), nil
}
```

### Фаза 2 — k-way merge через `container/heap`

```go
type runReader struct {
    r   *bufio.Reader
    f   *os.File
    src int
}

func (rr *runReader) next() (int64, bool, error) {
    var b [8]byte
    if _, err := io.ReadFull(rr.r, b[:]); err != nil {
        if errors.Is(err, io.EOF) {
            return 0, false, nil
        }
        return 0, false, err
    }
    return int64(binary.LittleEndian.Uint64(b[:])), true, nil
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
            for _, rr := range readers {
                rr.f.Close()
            }
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
        v, ok, err := rr.next()
        if err != nil {
            return err
        }
        if ok {
            heap.Push(h, hItem{val: v, src: rr.src})
        }
    }

    out, err := os.Create(outPath)
    if err != nil {
        return err
    }
    w := bufio.NewWriterSize(out, 1<<20)

    var b [8]byte
    for h.Len() > 0 {
        it := heap.Pop(h).(hItem)
        binary.LittleEndian.PutUint64(b[:], uint64(it.val))
        if _, err := w.Write(b[:]); err != nil {
            out.Close()
            return err
        }
        // подтянуть следующий элемент из того же рана
        v, ok, err := readers[it.src].next()
        if err != nil {
            out.Close()
            return err
        }
        if ok {
            heap.Push(h, hItem{val: v, src: it.src})
        }
    }
    if err := w.Flush(); err != nil {
        out.Close()
        return err
    }
    return out.Close()
}
```

`io.EOF` на границе записи означает нормальное завершение рана, а
`io.ErrUnexpectedEOF` означает повреждённый бинарный файл. Объединять оба случая
в `ok = false` нельзя: сортировка молча потеряет хвост данных.

---

## Многопроходное слияние и параллелизм

Тысячи ранов нельзя слить за один проход — под буфер каждого рана нужна память. При RAM 128 MB и буфере ~1 MB (чтобы чтение с HDD было последовательным) `k ≈ 100` ранов за проход. Решение — **многопроходное слияние группами**:

```go
// externalSort: полный пайплайн. maxFanIn = сколько ранов сливаем за проход (~RAM/буфер).
func externalSort(inputPath, outputPath, tmpDir string, chunkInts, maxFanIn int) error {
    if chunkInts <= 0 || maxFanIn < 2 {
        return errors.New("external sort: chunkInts must be positive and maxFanIn must be at least 2")
    }

    runs, err := generateRuns(inputPath, tmpDir, chunkInts) // Фаза 1 (параллелится)
    if err != nil {
        return err
    }

    if len(runs) == 0 {
        return os.WriteFile(outputPath, nil, 0o644)
    }

    for len(runs) > 1 {
        var next []string
        // бьём раны на группы по maxFanIn и сливаем каждую в новый ран
        for i := 0; i < len(runs); i += maxFanIn {
            group := runs[i:min(i+maxFanIn, len(runs))]
            merged, err := os.CreateTemp(tmpDir, "merge-*.bin")
            if err != nil {
                return err
            }
            if err := merged.Close(); err != nil {
                os.Remove(merged.Name())
                return err
            }
            if err := mergeRuns(group, merged.Name()); err != nil {
                os.Remove(merged.Name())
                return err
            }
            next = append(next, merged.Name())
            for _, r := range group {
                if err := os.Remove(r); err != nil {
                    return err
                }
            }
        }
        runs = next
    }
    // runs[0] — отсортированный бинарь; при необходимости конвертируем обратно в текст
    return binToText(runs[0], outputPath)
}

func binToText(inputPath, outputPath string) error {
    input, err := os.Open(inputPath)
    if err != nil {
        return err
    }
    defer input.Close()

    output, err := os.Create(outputPath)
    if err != nil {
        return err
    }

    reader := bufio.NewReaderSize(input, 1<<20)
    writer := bufio.NewWriterSize(output, 1<<20)
    var raw [8]byte

    for {
        _, err := io.ReadFull(reader, raw[:])
        if errors.Is(err, io.EOF) {
            break
        }
        if err != nil {
            output.Close()
            return err
        }

        value := int64(binary.LittleEndian.Uint64(raw[:]))
        if _, err := writer.WriteString(strconv.FormatInt(value, 10) + "\n"); err != nil {
            output.Close()
            return err
        }
    }

    if err := writer.Flush(); err != nil {
        output.Close()
        return err
    }
    return output.Close()
}
```

Оптимизации, которые стоит назвать:

| Приём | Зачем |
|---|---|
| **Бинарные временные файлы** (8 байт/число) | меньше I/O и места, нет парсинга строк на каждом проходе |
| **Большие последовательные буферы** (≥1 MB) | HDD быстр на sequential, медленный на random — избегаем seek'ов |
| **Многоядерная Фаза 1** | сортировку независимых кусков раздаём горутинам/воркер-пулу |
| **Параллельные группы слияния** | полезны только пока не насыщена полоса диска; на одном HDD могут ухудшить последовательность I/O |
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
    if err := os.WriteFile(in, []byte(sb.String()), 0o644); err != nil {
        t.Fatal(err)
    }
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

func readInts(t *testing.T, path string) []int64 {
    t.Helper()

    file, err := os.Open(path)
    if err != nil {
        t.Fatal(err)
    }
    defer file.Close()

    var values []int64
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        value, err := strconv.ParseInt(scanner.Text(), 10, 64)
        if err != nil {
            t.Fatal(err)
        }
        values = append(values, value)
    }
    if err := scanner.Err(); err != nil {
        t.Fatal(err)
    }
    return values
}
```

Маленькие `chunkInts` и `maxFanIn` создают сто ранов и несколько проходов на
небольшом наборе. Отдельно нужны тесты пустого файла, обрезанного бинарного рана,
невалидной конфигурации и ошибки записи.

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

Пусть после генерации получилось `R = 4 000` ранов, а за один раз можно слить
`k = 64`. Число проходов слияния:

```text
после прохода 1: ceil(4000 / 64) = 63 рана
после прохода 2: ceil(63 / 64) = 1 ран
```

Получаются один проход генерации и два прохода слияния. Каждый merge-pass читает
и записывает весь бинарный объём ранов. Время оценивается как
`total_bytes_read_and_written / measured_sequential_bandwidth`; подставлять
размер текстового входа вместо бинарного временного объёма нельзя.

К fan-in применяются сразу два ограничения: память на входные и выходной буферы,
а также лимит открытых файлов. Параллелизм полезен до насыщения диска, после чего
он лишь добавляет конкурирующие I/O-потоки.

---

## Возможные расширения

- **Распределённая сортировка** — данные на нескольких машинах: partition по
  диапазону, локальная сортировка и запись разделов в глобальном порядке.
- **SSD вместо HDD** — random I/O дешевеет, можно агрессивнее по fan-in и меньше думать о sequential.
- **Сортировка строк/записей переменной длины** — раны хранят offset’ы; усложняется формат, но схема та же.
- **Top-N вместо полной сортировки** — если нужны не все, а N наибольших, хватит heap размера N за один проход (см. родственную задачу).

---

## Interview-ready answer

**1. Как отсортировать данные, которые не помещаются в RAM?**

- Фаза 1 — читать ограниченные куски, сортировать каждый в памяти и записывать
  отсортированные раны.
- Фаза 2 — держать по одному текущему элементу каждого входного рана в min-heap
  и последовательно писать глобальный минимум.
- Ограничение — если все раны нельзя открыть и буферизовать одновременно,
  выполнять k-way merge в несколько проходов.

**2. Как выбрать размер куска и fan-in?**

- Кусок — учитывает не только массив `int64`, но и runtime, парсер, буферы и
  параллельные workers.
- Fan-in — ограничен памятью на буферы и числом открытых файлов.
- Проверка — реальные значения выбираются по измеренной памяти и
  последовательной пропускной способности диска.

**3. Какие ошибки приводят к потере данных?**

- Чтение — `io.ErrUnexpectedEOF` нельзя трактовать как нормальный конец рана.
- Запись — ошибки `Flush` и `Close` являются частью результата операции.
- Cleanup — входные раны удаляются только после успешной записи следующего
  уровня.
- Edge cases — пустой вход и невалидный `maxFanIn` обрабатываются отдельно.

---

## Связки с другими темами

- [Top-K Elements](./02-top-k.md) — тот же мотив «данные не влезают в память», min-heap, O(K) памяти.
- [Sorting and heap](../../../16-algorithms-and-data-structures/06-sorting-and-heap.md) — теория merge sort и кучи.
- [Stack and heap / иерархия памяти](../../../01-go-core/memory-internals/01-stack-and-heap.md) — почему RAM ограничена и при чём тут диск.
- [Memory hierarchy](../../../10-devops-and-observability/hardware-and-os/02-memory-hierarchy.md) — RAM vs диск, цена random I/O.
- [Streaming aggregation](../streams/03-streaming-aggregation.md) — обработка потоков с ограниченной памятью.
