package course_schedule

// LeetCode 207. Course Schedule (Medium)
// https://leetcode.com/problems/course-schedule/
//
// Задача: дано numCourses курсов и список зависимостей prerequisites, где
// [a, b] означает «чтобы пройти a, нужно сначала пройти b». Вернуть true, если
// можно пройти все курсы (т.е. в графе зависимостей нет цикла).
//
//	numCourses = 2, prerequisites = [[1,0]]         -> true   (0 затем 1)
//	numCourses = 2, prerequisites = [[1,0],[0,1]]   -> false  (цикл 0<->1)
//
// Идея — топологическая сортировка по Кану (BFS):
//   - строим граф b -> a и считаем входящие степени каждого курса;
//   - кладём в очередь курсы без зависимостей (входящая степень 0);
//   - «снимаем» их, уменьшая входящие степени соседей; обнулившиеся — в очередь;
//   - если удалось обработать все курсы, цикла нет.
//
// Сложность: O(V + E) по времени и памяти.
func CanFinish(numCourses int, prerequisites [][]int) bool {
	adj := make([][]int, numCourses)
	indegree := make([]int, numCourses)
	for _, p := range prerequisites {
		course, prereq := p[0], p[1]
		adj[prereq] = append(adj[prereq], course)
		indegree[course]++
	}

	queue := make([]int, 0, numCourses)
	for course := 0; course < numCourses; course++ {
		if indegree[course] == 0 {
			queue = append(queue, course)
		}
	}

	processed := 0
	for len(queue) > 0 {
		course := queue[0]
		queue = queue[1:]
		processed++

		for _, next := range adj[course] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	return processed == numCourses
}
