package main

type MergerIDSet map[int]bool

func (mset MergerIDSet) Add(id int) {
	mset[id] = true
}

func (mset MergerIDSet) Has(id int) bool {
	if _, ok := mset[id]; ok {
		return true
	}
	return false
}

func (mset MergerIDSet) Delete(id int) {
	delete(mset, id)
}

func (mset MergerIDSet) ToSlice() []int {
	var ids []int
	for id := range mset {
		ids = append(ids, id)
	}
	return ids
}
