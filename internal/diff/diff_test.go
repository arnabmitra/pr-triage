package diff

import "testing"

const sample = `diff --git a/a.go b/a.go
index 111..222 100644
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@ package a
 package a
-func Old() {}
+func New() {}
+func Extra() {}
@@ -20,2 +21,2 @@ func x() {
-	return 1
+	return 2
diff --git a/gone.sql b/gone.sql
deleted file mode 100644
index 333..000
--- a/gone.sql
+++ /dev/null
@@ -1 +0,0 @@
-DROP TABLE x;
\ No newline at end of file
`

func TestParse(t *testing.T) {
	hunks := Parse(sample)
	if len(hunks) != 3 {
		t.Fatalf("got %d hunks, want 3", len(hunks))
	}
	if h := hunks[0]; h.File != "a.go" || h.Added != 2 || h.Removed != 1 || h.StartLine != 1 {
		t.Errorf("hunk 0 = %+v", h)
	}
	if h := hunks[1]; h.StartLine != 21 || h.ID != "h2" {
		t.Errorf("hunk 1 = %+v", h)
	}
	if h := hunks[2]; h.File != "gone.sql" || !h.Deleted || h.Removed != 1 {
		t.Errorf("hunk 2 = %+v", h)
	}
	if hunks[0].Key == hunks[1].Key {
		t.Error("keys should differ")
	}
}
