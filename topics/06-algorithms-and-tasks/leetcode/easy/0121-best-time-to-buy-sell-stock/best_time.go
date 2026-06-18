package best_time

// LeetCode 121. Best Time to Buy and Sell Stock (Easy)
// https://leetcode.com/problems/best-time-to-buy-and-sell-stock/
//
// Задача: prices[i] — цена акции в день i. Купить можно один раз и продать
// позже. Вернуть максимальную возможную прибыль, либо 0, если её нет.
//
//	[7,1,5,3,6,4] -> 5   (купить за 1, продать за 6)
//	[7,6,4,3,1]   -> 0   (цена только падает)
//
// Идея: одним проходом держим минимальную цену слева; для каждого дня прибыль =
// текущая цена минус этот минимум, обновляем лучший результат.
//
// Сложность: O(n) по времени, O(1) по памяти.
func MaxProfit(prices []int) int {
	if len(prices) == 0 {
		return 0
	}
	minPrice := prices[0]
	best := 0
	for _, p := range prices[1:] {
		if p < minPrice {
			minPrice = p
		} else if profit := p - minPrice; profit > best {
			best = profit
		}
	}
	return best
}
