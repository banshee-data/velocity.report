package lidar

import (
	"testing"
)

func TestHungarianAssign_Empty(t *testing.T) {
	result := hungarianAssign(nil)
	if result != nil {
		t.Errorf("expected nil for empty cost matrix, got %v", result)
	}
}

func TestHungarianAssign_SingleElement(t *testing.T) {
	cost := [][]float32{{5.0}}
	result := hungarianAssign(cost)
	if len(result) != 1 || result[0] != 0 {
		t.Errorf("expected [0], got %v", result)
	}
}

func TestHungarianAssign_SquareOptimal(t *testing.T) {
	// Classic 3x3 assignment problem:
	//   [1 2 3]     Optimal: row0→col0 (1), row1→col1 (4), row2→col2 (5) = 10
	//   [4 4 6]     NOT: row0→col0 (1), row1→col2 (6), row2→col1 (8) = 15
	//   [9 8 5]
	cost := [][]float32{
		{1, 2, 3},
		{4, 4, 6},
		{9, 8, 5},
	}
	result := hungarianAssign(cost)

	if len(result) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(result))
	}

	totalCost := float32(0)
	for i, j := range result {
		if j < 0 {
			t.Errorf("row %d unassigned", i)
			continue
		}
		totalCost += cost[i][j]
	}

	if totalCost != 10.0 {
		t.Errorf("expected optimal cost 10, got %v (assignments: %v)", totalCost, result)
	}
}

func TestHungarianAssign_Forbidden(t *testing.T) {
	// Row 1 has no reachable column (all forbidden).
	cost := [][]float32{
		{1, 2},
		{float32(hungarianlnf), float32(hungarianlnf)},
	}
	result := hungarianAssign(cost)

	if len(result) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(result))
	}
	// Row 0 should be assigned.
	if result[0] < 0 {
		t.Errorf("row 0 should be assigned, got %d", result[0])
	}
	// Row 1 must be unassigned (all costs forbidden).
	if result[1] != -1 {
		t.Errorf("row 1 should be unassigned (-1), got %d", result[1])
	}
}

func TestHungarianAssign_MoreRowsThanCols(t *testing.T) {
	// 3 rows, 2 cols → one row must go unassigned.
	cost := [][]float32{
		{1, 10},
		{10, 1},
		{5, 5},
	}
	result := hungarianAssign(cost)

	if len(result) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(result))
	}

	assigned := 0
	for _, j := range result {
		if j >= 0 {
			assigned++
		}
	}
	if assigned != 2 {
		t.Errorf("expected exactly 2 assigned rows, got %d (result: %v)", assigned, result)
	}

	// Optimal: row0→col0(1), row1→col1(1) = 2
	totalCost := float32(0)
	for i, j := range result {
		if j >= 0 {
			totalCost += cost[i][j]
		}
	}
	if totalCost != 2.0 {
		t.Errorf("expected optimal cost 2, got %v (assignments: %v)", totalCost, result)
	}
}

func TestHungarianAssign_MoreColsThanRows(t *testing.T) {
	// 2 rows, 3 cols → all rows assigned.
	cost := [][]float32{
		{10, 1, 5},
		{5, 10, 1},
	}
	result := hungarianAssign(cost)

	if len(result) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(result))
	}

	// Both rows should be assigned.
	for i, j := range result {
		if j < 0 {
			t.Errorf("row %d unassigned", i)
		}
	}

	// Optimal: row0→col1(1), row1→col2(1) = 2
	totalCost := float32(0)
	for i, j := range result {
		if j >= 0 {
			totalCost += cost[i][j]
		}
	}
	if totalCost != 2.0 {
		t.Errorf("expected optimal cost 2, got %v (assignments: %v)", totalCost, result)
	}
}

func TestHungarianAssign_AllZeroCost(t *testing.T) {
	cost := [][]float32{
		{0, 0},
		{0, 0},
	}
	result := hungarianAssign(cost)

	if len(result) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(result))
	}

	// Both should be assigned, and to different columns.
	if result[0] == result[1] {
		t.Errorf("both rows assigned to same column: %v", result)
	}
}

func TestHungarianAssign_NoColumns(t *testing.T) {
	cost := [][]float32{
		{},
		{},
	}
	result := hungarianAssign(cost)

	if len(result) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(result))
	}
	for i, j := range result {
		if j != -1 {
			t.Errorf("row %d should be -1 (no columns), got %d", i, j)
		}
	}
}

func TestHungarianAssign_LargerOptimality(t *testing.T) {
	// 4x4 problem with known optimal.
	// Optimal assignment: (0,3)=1, (1,2)=2, (2,1)=3, (3,0)=4 → total=10
	cost := [][]float32{
		{10, 5, 7, 1},
		{8, 9, 2, 6},
		{7, 3, 11, 5},
		{4, 12, 8, 9},
	}
	result := hungarianAssign(cost)

	totalCost := float32(0)
	for i, j := range result {
		if j < 0 {
			t.Errorf("row %d unassigned in 4×4 problem", i)
			continue
		}
		totalCost += cost[i][j]
	}
	if totalCost != 10.0 {
		t.Errorf("expected optimal cost 10, got %v (assignments: %v)", totalCost, result)
	}
}
