package main

func main() {
	chunks := splitAndSortChunks("bigfile.txt")
	mergeChunks(chunks, "sorted_output.txt")
}
