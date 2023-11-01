package xmfile

type objectPool[T any] struct {
	lists    []objectPoolList[T]
	listSize int
}

type objectPoolList[T any] struct {
	data []T
	used int
}

func (l *objectPoolList[T]) Reset() {
	l.used = 0
}

func (l *objectPoolList[T]) ElemsAvailable() int {
	return len(l.data) - l.used
}

func (l *objectPoolList[T]) MakeSlice(n int) []T {
	slice := l.data[l.used : l.used+n]
	l.used += n
	return slice
}

func initObjectPool[T any](p *objectPool[T], listSize int) {
	p.lists = make([]objectPoolList[T], 0, 6)
	p.listSize = listSize
}

func (p *objectPool[T]) Reset() {
	for i := range p.lists {
		l := &p.lists[i]
		l.Reset()
	}
}

func (p *objectPool[T]) MakeSlice(n int) []T {
	if n > p.listSize {
		// This memory block can't fit in any of the lists,
		// so allocate it here right away.
		return make([]T, n)
	}

	// Try to allocate this slice from an existing list.
	for i := range p.lists {
		l := &p.lists[i]
		if l.ElemsAvailable() >= n {
			return l.MakeSlice(n)
		}
	}

	// None of the lists can allocate this memory.
	// Perhaps we can add another list?
	if len(p.lists) < cap(p.lists) {
		// Since we already checked that n<maxSize,
		// we can be sure that this new list will have enough
		// space to make this allocation.
		p.lists = append(p.lists, objectPoolList[T]{
			data: make([]T, p.listSize),
		})
		l := &p.lists[len(p.lists)-1]
		return l.MakeSlice(n)
	}

	// This pool is out of space.
	// We'll have to allocate this memory without a pool.
	return make([]T, n)
}
