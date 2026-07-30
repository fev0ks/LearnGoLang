// Форма hash-trie при росте числа ключей.
//
// ВАЖНО: это МОДЕЛЬ, а не внутренности реальной sync.Map. Настоящее дерево
// недоступно извне, а runtime хэширует ключи своей функцией со случайным
// seed на каждую map. Здесь используется собственный hash, поэтому конкретные
// номера ветвей у реальной sync.Map будут другими.
//
// Совпадает то, что от hash не зависит: правило раскладки по четыре бита на
// уровень, момент появления нового уровня и статистика формы дерева.
package main

import (
	"flag"
	"fmt"
	"math"
	"sort"
)

const (
	nChildren  = 16 // 4 бита на уровень
	nibbleBits = 4
	maxLevels  = 64 / nibbleBits // для 64-битного hash
)

// node повторяет разделение из реализации: узел либо ячейка с ключом,
// либо промежуточный узел с 16 ветвями.
type node struct {
	isEntry  bool
	key      string
	hash     uint64
	children [nChildren]*node
}

// nibble возвращает номер ветви на заданном уровне: очередные четыре бита,
// начиная со старших. Уровень 0 — это выбор в root.
func nibble(hash uint64, level int) int {
	shift := 64 - nibbleBits*(level+1)
	return int((hash >> shift) & (nChildren - 1))
}

type trie struct {
	root *node

	indirectNodes int // сколько промежуточных узлов создано
	fullCollision int // сколько ключей упёрлись в overflow chain
}

func (t *trie) insert(key string, hash uint64) {
	if t.root == nil {
		t.root = &node{}
		t.indirectNodes++
	}
	parent, level := t.root, 0

	for {
		idx := nibble(hash, level)
		child := parent.children[idx]

		switch {
		case child == nil:
			// Свободная ветвь: ячейка публикуется здесь, уровень не растёт.
			parent.children[idx] = &node{isEntry: true, key: key, hash: hash}
			return

		case !child.isEntry:
			// Промежуточный узел: спуск глубже.
			parent, level = child, level+1

		case child.key == key:
			// Тот же ключ: замена значения, форма дерева не меняется.
			return

		default:
			// Занято другой ячейкой — ветвь надо углубить.
			t.expand(parent, idx, child, key, hash, level+1)
			return
		}
	}
}

// expand создаёт уровни, пока очередные нибблы двух ключей совпадают.
// Реализация не сжимает путь, поэтому общий префикс hash превращается
// в цепочку узлов с единственным потомком.
func (t *trie) expand(parent *node, idx int, old *node, key string, hash uint64, level int) {
	for level < maxLevels {
		branch := &node{}
		t.indirectNodes++
		parent.children[idx] = branch

		oldIdx, newIdx := nibble(old.hash, level), nibble(hash, level)
		if oldIdx != newIdx {
			branch.children[oldIdx] = old
			branch.children[newIdx] = &node{isEntry: true, key: key, hash: hash}
			return
		}

		// Нибблы совпали: нужен ещё уровень.
		parent, idx = branch, oldIdx
		level++
	}

	// Биты кончились: полное совпадение hash, дальше overflow chain.
	parent.children[idx] = old
	t.fullCollision++
}

type stats struct {
	depths        []int
	indirectNodes int
	singleChild   int // промежуточные узлы ровно с одним потомком
	maxDepth      int
}

func (t *trie) stats() stats {
	s := stats{indirectNodes: t.indirectNodes}
	if t.root == nil {
		return s
	}
	var walk func(n *node, depth int)
	walk = func(n *node, depth int) {
		used := 0
		for _, c := range n.children {
			if c == nil {
				continue
			}
			used++
			if c.isEntry {
				s.depths = append(s.depths, depth+1)
				if depth+1 > s.maxDepth {
					s.maxDepth = depth + 1
				}
				continue
			}
			walk(c, depth+1)
		}
		if used == 1 {
			s.singleChild++
		}
	}
	walk(t.root, 0)
	return s
}

func (t *trie) print() {
	if t.root == nil {
		fmt.Println("(пустое дерево)")
		return
	}
	printNode(t.root, "")
}

func printNode(n *node, prefix string) {
	var used []int
	for i, c := range n.children {
		if c != nil {
			used = append(used, i)
		}
	}
	for pos, i := range used {
		last := pos == len(used)-1
		branch, next := "├──", "│   "
		if last {
			branch, next = "└──", "    "
		}
		c := n.children[i]
		if c.isEntry {
			fmt.Printf("%s%s [%X] entry %q  hash=%016x\n", prefix, branch, i, c.key, c.hash)
			continue
		}
		fmt.Printf("%s%s [%X] indirect\n", prefix, branch, i)
		printNode(c, prefix+next)
	}
}

// hashKey — обычный FNV-1a с финализатором splitmix64.
// Детерминирован между запусками, чтобы вывод примера был воспроизводим.
func hashKey(key string, seed uint64) uint64 {
	h := uint64(14695981039346656037) ^ seed
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= 1099511628211
	}
	// Перемешивание: без него старшие биты у похожих ключей слабо различаются,
	// а номер ветви в root берётся именно из старших бит.
	h ^= h >> 30
	h *= 0xbf58476d1ce4e5b9
	h ^= h >> 27
	h *= 0x94d049bb133111eb
	h ^= h >> 31
	return h
}

func main() {
	n := flag.Int("n", 10, "число ключей")
	seed := flag.Uint64("seed", 1, "seed hash-функции")
	pattern := flag.String("key", "user:%d", "шаблон ключа с одним %d")
	show := flag.Int("show", 32, "печатать дерево, если ключей не больше этого числа")
	flag.Parse()

	t := &trie{}
	for i := 1; i <= *n; i++ {
		key := fmt.Sprintf(*pattern, i)
		t.insert(key, hashKey(key, *seed))
	}

	if *n <= *show {
		fmt.Printf("Дерево для %d ключей (root, дальше по четыре бита hash):\n\n", *n)
		t.print()
		fmt.Println()
	}

	s := t.stats()
	report(*n, s, t.fullCollision)
}

func report(n int, s stats, fullCollision int) {
	if len(s.depths) == 0 {
		fmt.Println("ключей нет")
		return
	}
	sort.Ints(s.depths)

	sum := 0
	hist := map[int]int{}
	for _, d := range s.depths {
		sum += d
		hist[d]++
	}
	avg := float64(sum) / float64(len(s.depths))

	fmt.Printf("ключей:                     %d\n", n)
	fmt.Printf("глубина: средняя %.2f, максимальная %d, теоретический минимум log16(n)=%.2f\n",
		avg, s.maxDepth, math.Log(float64(n))/math.Log(nChildren))
	fmt.Printf("промежуточных узлов:        %d\n", s.indirectNodes)
	fmt.Printf("из них с одним потомком:    %d (путь не сжимается)\n", s.singleChild)
	fmt.Printf("память под указатели узлов: ~%d КБ (%d узлов × 16 указателей × 8 байт)\n",
		s.indirectNodes*nChildren*8/1024, s.indirectNodes)
	if fullCollision > 0 {
		fmt.Printf("полных коллизий hash:       %d (ушли в overflow chain)\n", fullCollision)
	}

	fmt.Println("\nраспределение глубины:")
	depths := make([]int, 0, len(hist))
	for d := range hist {
		depths = append(depths, d)
	}
	sort.Ints(depths)
	for _, d := range depths {
		count := hist[d]
		bar := count * 40 / len(s.depths)
		if bar == 0 {
			bar = 1
		}
		fmt.Printf("  уровень %2d: %6d ключей %5.1f%%  %s\n",
			d, count, 100*float64(count)/float64(len(s.depths)), repeat("#", bar))
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for range n {
		out = append(out, s...)
	}
	return string(out)
}
