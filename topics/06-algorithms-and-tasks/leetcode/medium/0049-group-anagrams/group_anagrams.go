package group_anagrams

import "sort"

// LeetCode 49. Group Anagrams (Medium)
// https://leetcode.com/problems/group-anagrams/
//
// Задача: сгруппировать слова, являющиеся анаграммами друг друга.
//
//	["eat","tea","tan","ate","nat","bat"]
//	  -> [["bat"],["nat","tan"],["ate","eat","tea"]]   (порядок групп не важен)
//
// Идея: у анаграмм совпадает отсортированный набор букв — используем его как
// ключ map'а, под которым копим исходные слова.
//
// Сложность: O(n * k log k), где n — число слов, k — их длина (на сортировку
// каждого ключа).
func GroupAnagrams(strs []string) [][]string {
	groups := make(map[string][]string)
	for _, s := range strs {
		key := sortedKey(s)
		groups[key] = append(groups[key], s)
	}

	res := make([][]string, 0, len(groups))
	for _, group := range groups {
		res = append(res, group)
	}
	return res
}

// sortedKey возвращает буквы слова в отсортированном порядке — общий ключ
// анаграмм.
func sortedKey(s string) string {
	b := []byte(s)
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return string(b)
}
