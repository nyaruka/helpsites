package models

import "github.com/lib/pq"

func pqStringArray(vs []string) pq.StringArray {
	return pq.StringArray(vs)
}

func pqIntArray(ids []ArticleID) pq.Int64Array {
	arr := make(pq.Int64Array, len(ids))
	for i, id := range ids {
		arr[i] = int64(id)
	}
	return arr
}
