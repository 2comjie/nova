package diff_sync

import "testing"

func TestAOF(t *testing.T) {
	dir := t.TempDir()

	first, err := openAOF(dir, aofSegmentSize)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Append(1001, 1, false, []byte("proto diff")) {
		t.Fatal("AOF 已满")
	}
	offset := first.offset
	first.Close()

	second, err := openAOF(dir, aofSegmentSize)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if second.offset != offset {
		t.Fatalf("offset=%d, want=%d", second.offset, offset)
	}
}
