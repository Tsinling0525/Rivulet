package infra

import "github.com/Tsinling0525/rivulet/model"

func cloneItemsMap(src map[model.ID]model.Items) map[model.ID]model.Items {
	if src == nil {
		return nil
	}
	out := make(map[model.ID]model.Items, len(src))
	for k, items := range src {
		clonedItems := make(model.Items, 0, len(items))
		for _, item := range items {
			cloned := model.Item{}
			for field, value := range item {
				cloned[field] = value
			}
			clonedItems = append(clonedItems, cloned)
		}
		out[k] = clonedItems
	}
	return out
}
