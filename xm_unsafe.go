package xm

import (
	"unsafe"
)

func moduleSize(m *module) uint {
	memoryUsage := 0
	for _, inst := range m.instruments {
		memoryUsage += len(inst.samples) * 2
	}
	for _, p := range m.patterns {
		memoryUsage += int(unsafe.Sizeof(pattern{}))
		memoryUsage += len(p.notes) * 2
	}
	memoryUsage += len(m.noteTab) * int(unsafe.Sizeof(patternNote{}))
	memoryUsage += len(m.effectTab) * int(unsafe.Sizeof(noteEffect{}))

	return uint(memoryUsage)
}
