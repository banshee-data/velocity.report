package lidar

import (
	"sync"
	"testing"
)

func TestRingBuffer_Push(t *testing.T) {
	rb := &RingBuffer[int]{
		Items:    make([]int, 5),
		Capacity: 5,
	}

	// Test pushing items
	for i := 0; i < 5; i++ {
		if !rb.Push(i) {
			t.Errorf("Push(%d) failed, expected success", i)
		}
	}

	// Verify size
	if rb.Len() != 5 {
		t.Errorf("Size = %d, want 5", rb.Len())
	}

	// Test pushing when full
	if rb.Push(99) {
		t.Error("Push succeeded when buffer was full, expected false")
	}
}

func TestRingBuffer_Pop(t *testing.T) {
	rb := &RingBuffer[int]{
		Items:    make([]int, 5),
		Capacity: 5,
	}

	// Test popping from empty buffer
	_, ok := rb.Pop()
	if ok {
		t.Error("Pop succeeded on empty buffer, expected false")
	}

	// Push items and pop them
	for i := 0; i < 3; i++ {
		rb.Push(i * 10)
	}

	for i := 0; i < 3; i++ {
		val, ok := rb.Pop()
		if !ok {
			t.Errorf("Pop %d failed", i)
		}
		if val != i*10 {
			t.Errorf("Pop %d = %d, want %d", i, val, i*10)
		}
	}

	// Verify buffer is empty
	if rb.Len() != 0 {
		t.Errorf("Size = %d, want 0", rb.Len())
	}
}

func TestRingBuffer_Wraparound(t *testing.T) {
	rb := &RingBuffer[string]{
		Items:    make([]string, 3),
		Capacity: 3,
	}

	// Fill buffer
	rb.Push("a")
	rb.Push("b")
	rb.Push("c")

	// Pop one
	val, _ := rb.Pop()
	if val != "a" {
		t.Errorf("First pop = %s, want 'a'", val)
	}

	// Push another (should wrap around)
	rb.Push("d")

	// Pop and verify order
	expected := []string{"b", "c", "d"}
	for i, exp := range expected {
		val, ok := rb.Pop()
		if !ok {
			t.Errorf("Pop %d failed", i)
		}
		if val != exp {
			t.Errorf("Pop %d = %s, want %s", i, val, exp)
		}
	}
}

func TestRingBuffer_Len(t *testing.T) {
	rb := &RingBuffer[float64]{
		Items:    make([]float64, 10),
		Capacity: 10,
	}

	if rb.Len() != 0 {
		t.Errorf("Initial size = %d, want 0", rb.Len())
	}

	for i := 0; i < 5; i++ {
		rb.Push(float64(i))
		if rb.Len() != i+1 {
			t.Errorf("After push %d, size = %d, want %d", i, rb.Len(), i+1)
		}
	}

	for i := 0; i < 5; i++ {
		rb.Pop()
		if rb.Len() != 4-i {
			t.Errorf("After pop %d, size = %d, want %d", i, rb.Len(), 4-i)
		}
	}
}

func TestRingBuffer_Concurrent(t *testing.T) {
	rb := &RingBuffer[int]{
		Items:    make([]int, 100),
		Capacity: 100,
	}

	var wg sync.WaitGroup
	pushCount := 50
	popCount := 30

	// Concurrent pushes
	for i := 0; i < pushCount; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			rb.Push(val)
		}(i)
	}

	wg.Wait()

	// Verify we have items
	if rb.Len() < 1 {
		t.Error("No items after concurrent pushes")
	}

	initialSize := rb.Len()

	// Concurrent pops
	for i := 0; i < popCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.Pop()
		}()
	}

	wg.Wait()

	// Verify size decreased
	finalSize := rb.Len()
	if finalSize >= initialSize {
		t.Errorf("Size didn't decrease after pops: initial=%d, final=%d", initialSize, finalSize)
	}
}

func TestRingBuffer_MemoryCleanup(t *testing.T) {
	rb := &RingBuffer[*string]{
		Items:    make([]*string, 3),
		Capacity: 3,
	}

	// Push some items
	s1, s2, s3 := "one", "two", "three"
	rb.Push(&s1)
	rb.Push(&s2)
	rb.Push(&s3)

	// Pop an item - verify it clears the reference
	rb.Pop()

	// Check that the first slot was cleared (zero value)
	if rb.Items[0] != nil {
		t.Error("Item reference was not cleared after pop")
	}
}
