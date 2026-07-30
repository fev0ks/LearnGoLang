// CRUD над sync.Map: все методы по очереди, с акцентом на места,
// где поведение отличается от обычной map.
package main

import (
	"fmt"
	"sync"
)

type User struct {
	Name  string
	Email string
}

func (u User) String() string {
	if u.Email == "" {
		return u.Name + " <без email>"
	}
	return u.Name + " <" + u.Email + ">"
}

func main() {
	// Нулевое значение готово к работе: make не нужен.
	// Копировать sync.Map после первого использования нельзя,
	// поэтому в реальном коде её держат полем и передают по указателю.
	var users sync.Map

	section("CREATE")
	create(&users)

	section("READ")
	read(&users)

	section("UPDATE")
	update(&users)

	section("UPDATE одного поля через CAS")
	updateField(&users, "u1", "alice@corp.example")

	section("DELETE")
	remove(&users)

	section("LIST через Range")
	list(&users)

	section("Конкурентный доступ")
	concurrent()
}

func create(users *sync.Map) {
	// Store — безусловная вставка или перезапись.
	users.Store("u1", &User{Name: "alice", Email: "a@example.com"})
	users.Store("u2", &User{Name: "bob"})

	// LoadOrStore — вставить, только если ключа ещё нет.
	// loaded=false означает "сохранён наш объект".
	actual, loaded := users.LoadOrStore("u3", &User{Name: "carol"})
	fmt.Printf("LoadOrStore(u3): loaded=%v value=%v\n", loaded, actual)

	// Повторный вызов с тем же ключом наш объект уже не сохранит.
	actual, loaded = users.LoadOrStore("u3", &User{Name: "не попадёт в map"})
	fmt.Printf("LoadOrStore(u3): loaded=%v value=%v\n", loaded, actual)
}

func read(users *sync.Map) {
	// Load возвращает any — нужен type assertion.
	if v, ok := users.Load("u1"); ok {
		fmt.Printf("Load(u1): %v\n", v.(*User))
	}

	// Промах.
	if _, ok := users.Load("u-404"); !ok {
		fmt.Println("Load(u-404): промах")
	}

	// Сохранённый nil возвращается с ok=true: отсутствие проверяют по ok.
	users.Store("u-nil", nil)
	v, ok := users.Load("u-nil")
	fmt.Printf("Load(u-nil): value=%v ok=%v <- не по value==nil\n", v, ok)
	users.Delete("u-nil")
}

func update(users *sync.Map) {
	// Store поверх существующего ключа заменяет значение.
	users.Store("u2", &User{Name: "bob", Email: "b@example.com"})

	// Swap возвращает прежнее значение.
	prev, loaded := users.Swap("u2", &User{Name: "bob", Email: "bob@corp.example"})
	fmt.Printf("Swap(u2): loaded=%v прежнее=%v\n", loaded, prev.(*User))

	// Swap отсутствующего ключа работает как вставка.
	prev, loaded = users.Swap("u9", &User{Name: "dave"})
	fmt.Printf("Swap(u9): loaded=%v прежнее=%v <- ключа не было\n", loaded, prev)
	users.Delete("u9")
}

// updateField меняет одно поле значения безопасно для конкурентного доступа.
//
// Прямое `old.Email = ...` по загруженному указателю было бы гонкой:
// sync.Map защищает ячейку с указателем, но не поля объекта за ним.
func updateField(users *sync.Map, key, email string) {
	for attempt := 1; ; attempt++ {
		cur, ok := users.Load(key)
		if !ok {
			fmt.Printf("updateField(%s): ключа нет\n", key)
			return
		}
		old := cur.(*User)

		// Копия, а не мутация общего объекта.
		updated := *old
		updated.Email = email

		// CAS сравнивает указатели: сработает только если между Load и CAS
		// никто не подменил значение.
		if users.CompareAndSwap(key, old, &updated) {
			fmt.Printf("updateField(%s): успех с попытки %d, теперь %v\n",
				key, attempt, &updated)
			return
		}
		// Кто-то вмешался — перечитать актуальное значение и повторить.
	}
}

func remove(users *sync.Map) {
	// Delete молча ничего не делает, если ключа нет.
	users.Delete("u-404")
	fmt.Println("Delete(u-404): ошибки нет, ключа просто не было")

	// LoadAndDelete сообщает, был ли ключ, и отдаёт значение.
	if v, loaded := users.LoadAndDelete("u3"); loaded {
		fmt.Printf("LoadAndDelete(u3): удалили %v\n", v.(*User))
	}

	// CompareAndDelete удаляет только совпадающее значение.
	cur, _ := users.Load("u2")
	if users.CompareAndDelete("u2", cur) {
		fmt.Printf("CompareAndDelete(u2): удалили %v\n", cur.(*User))
	}

	// Тот же вызов с устаревшим значением уже не сработает.
	if !users.CompareAndDelete("u1", &User{Name: "alice"}) {
		fmt.Println("CompareAndDelete(u1, другой указатель): false <- сравнение по указателю")
	}
}

func list(users *sync.Map) {
	// Range не даёт согласованного snapshot и не блокирует остальные методы.
	// Порядок обхода не определён.
	users.Store("u4", &User{Name: "erin"})
	users.Store("u5", &User{Name: "frank"})

	count := 0
	users.Range(func(key, value any) bool {
		count++
		fmt.Printf("  %v = %v\n", key, value.(*User))
		return true // false прекратил бы обход
	})
	fmt.Printf("посещено ключей: %d (Len у sync.Map нет)\n", count)
}

func concurrent() {
	// Счётчик на ключ: sync.Map отвечает за набор ключей,
	// а за значение каждого счётчика — свой atomic внутри объекта.
	var hits sync.Map // map[string]*counter
	var wg sync.WaitGroup

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, page := range []string{"/", "/about", "/"} {
				c := getCounter(&hits, page)
				c.inc()
			}
		}()
	}
	wg.Wait()

	hits.Range(func(key, value any) bool {
		fmt.Printf("  %v -> %d\n", key, value.(*counter).get())
		return true
	})
	fmt.Println("итого 300 инкрементов, ни одного потерянного")
}

func getCounter(m *sync.Map, key string) *counter {
	// Несколько goroutines могут создать несколько кандидатов,
	// но в map останется один — его и возвращает LoadOrStore всем.
	actual, _ := m.LoadOrStore(key, &counter{})
	return actual.(*counter)
}

type counter struct {
	mu sync.Mutex
	n  int
}

func (c *counter) inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *counter) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func section(title string) {
	fmt.Printf("\n=== %s\n", title)
}
