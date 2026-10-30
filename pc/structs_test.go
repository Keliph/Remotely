package main

import (
	"testing"
	"unsafe"
)

// SendInput silently does nothing if cbSize is wrong, so pin the sizes.
func TestInputStructSizes(t *testing.T) {
	want := uintptr(40)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 28
	}
	if s := unsafe.Sizeof(mouseINPUT{}); s != want {
		t.Fatalf("mouseINPUT = %d, want %d", s, want)
	}
	if s := unsafe.Sizeof(keyINPUT{}); s != want {
		t.Fatalf("keyINPUT = %d, want %d", s, want)
	}
	if o := unsafe.Offsetof(mouseINPUT{}.Mi); o != unsafe.Sizeof(uintptr(0)) {
		t.Fatalf("union offset = %d", o)
	}
}
