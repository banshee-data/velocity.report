package db

import (
	"strings"
	"testing"
)

// A scene's vantages live in exactly one file, a vantages.json at the scene
// root, which is what the published viewer fetches. The scene index once
// carried a column of them too, and because nothing ever copied that column
// into an export, editing it changed nothing a visitor could see while looking
// for all the world like it had. This fails if such a column returns.
func TestSceneTableHasNoVantageColumn(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	rows, err := db.DB.Query(`SELECT name FROM pragma_table_info('lidar_scenes')`)
	if err != nil {
		t.Fatalf("read lidar_scenes columns: %v", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(name), "vantage") {
			t.Errorf("lidar_scenes.%s stores vantages; vantages.json at the scene "+
				"root is the only place they belong", name)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(columns) == 0 {
		t.Fatal("lidar_scenes has no columns; the migration did not run")
	}
}
