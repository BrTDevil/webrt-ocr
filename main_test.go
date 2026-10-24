package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestSupported(t *testing.T) {
	for name, want := range map[string]bool{
		"a.PDF": true, "b.jpg": true, "c.JPEG": true, "d.webp": true, "e.tiff": true,
		"f.txt": false, "g.docx": false, "fara-extensie": false,
	} {
		if got := supported(name); got != want {
			t.Errorf("supported(%q) = %v, vrut %v", name, got, want)
		}
	}
}

func TestExpandArgs(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.pdf", "b.png", "c.txt", ".ascuns.jpg"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	os.Mkdir(filepath.Join(dir, "sub.pdf"), 0o755)

	got, err := expandArgs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.png")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandArgs = %v, vrut %v", got, want)
	}
	if _, err := expandArgs([]string{filepath.Join(dir, "lipsa.pdf")}); err == nil {
		t.Error("fișier inexistent acceptat")
	}
}

func TestRomanianCommas(t *testing.T) {
	if got := romanianCommas.Replace("Ştiinţă şi ţară ȘȚ"); got != "Știință și țară ȘȚ" {
		t.Errorf("romanianCommas = %q", got)
	}
}

// Rotirea cu 90° în sensul acelor de ceasornic (orientarea EXIF 6) mută colțul
// stânga-sus în dreapta-sus.
func TestApplyOrientation(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	src.Set(0, 0, color.NRGBA{255, 0, 0, 255})

	d := applyOrientation(src, 6)
	if b := d.Bounds(); b.Dx() != 2 || b.Dy() != 3 {
		t.Fatalf("dimensiuni %v, vrut 2x3", b)
	}
	if r, _, _, _ := d.At(1, 0).RGBA(); r == 0 {
		t.Error("pixelul roșu nu a ajuns în dreapta-sus")
	}
	if applyOrientation(src, 1) != image.Image(src) {
		t.Error("orientarea 1 trebuie să întoarcă imaginea neschimbată")
	}
}
