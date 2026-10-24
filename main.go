// webrt-ocr — recunoaște textul din PDF-uri și imagini și îl scrie într-un .txt cu același nume.
//
//	scan.pdf  → scan.txt
//	poza.jpg  → poza.txt
//
// PDF: paginile care au deja text (PDF generat digital) sunt citite direct, exact;
// doar paginile scanate trec prin OCR. Cu -force, toate paginile trec prin OCR.
// OCR-ul este făcut de tesseract, randarea paginilor de pdftoppm (poppler-utils).
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const version = "1.0.0"

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".tif": true, ".tiff": true,
	".bmp": true, ".gif": true, ".webp": true,
}

type config struct {
	lang    string
	out     string
	dpi     int
	psm     int
	force   bool
	marks   bool
	workers int
	noRot   bool
	osd     bool // pachetul osd e instalat → putem detecta orientarea
}

func main() {
	var cfg config
	flag.StringVar(&cfg.lang, "lang", "ron+eng", "limbile textului, format tesseract (ron, eng, ron+eng, deu...)")
	flag.StringVar(&cfg.out, "o", "", "folderul pentru .txt (implicit lângă fișierul sursă)")
	flag.IntVar(&cfg.dpi, "dpi", 300, "rezoluția la care se randează paginile PDF pentru OCR")
	flag.IntVar(&cfg.psm, "psm", 3, "modul de segmentare tesseract (3 = automat, 6 = un singur bloc de text, 4 = coloane/tabele)")
	flag.BoolVar(&cfg.force, "force", false, "OCR pe toate paginile PDF, chiar și pe cele care au deja text")
	flag.BoolVar(&cfg.marks, "pages", false, "marchează începutul fiecărei pagini PDF în .txt")
	flag.BoolVar(&cfg.noRot, "norotate", false, "nu detecta și nu corecta automat paginile/pozele întoarse")
	flag.IntVar(&cfg.workers, "j", runtime.NumCPU(), "câte pagini/imagini se procesează în paralel")
	showVersion := flag.Bool("version", false, "afișează versiunea")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Utilizare: webrt-ocr [opțiuni] fisier.pdf|imagine|folder ...\n\n")
		fmt.Fprintf(os.Stderr, "Scrie textul recunoscut în <nume>.txt, lângă fiecare fișier.\n")
		fmt.Fprintf(os.Stderr, "Un folder = toate PDF-urile și imaginile din el (fără subfoldere).\n\nOpțiuni:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("webrt-ocr", version)
		return
	}
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if cfg.dpi < 72 || cfg.dpi > 1200 {
		fail("-dpi trebuie să fie între 72 și 1200")
	}
	if cfg.psm < 0 || cfg.psm > 13 {
		fail("-psm trebuie să fie între 0 și 13")
	}
	cfg.workers = max(1, cfg.workers)
	osd, err := checkTools(cfg.lang)
	if err != nil {
		fail("%v", err)
	}
	cfg.osd = osd
	if !cfg.noRot && !osd {
		fmt.Fprintln(os.Stderr, "Atenție: pachetul tesseract-ocr-osd lipsește, rotirea automată e dezactivată")
	}

	files, err := expandArgs(flag.Args())
	if err != nil {
		fail("%v", err)
	}
	if len(files) == 0 {
		fail("niciun PDF sau imagine de procesat")
	}

	failed := 0
	for _, f := range files {
		if err := process(f, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Eroare %s: %v\n", f, err)
			failed++
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

// checkTools verifică tesseract (și limbile cerute) înainte de orice fișier;
// întoarce dacă modelul osd (detecția orientării) e disponibil.
func checkTools(lang string) (bool, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return false, fmt.Errorf("tesseract nu e instalat: sudo apt install tesseract-ocr tesseract-ocr-ron")
	}
	out, err := exec.Command("tesseract", "--list-langs").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("tesseract --list-langs: %s", strings.TrimSpace(string(out)))
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(out), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	for _, l := range strings.Split(lang, "+") {
		if !have[l] {
			return false, fmt.Errorf("limba %q nu e instalată pentru tesseract: sudo apt install tesseract-ocr-%s", l, l)
		}
	}
	return have["osd"], nil
}

func expandArgs(args []string) ([]string, error) {
	var files []string
	for _, a := range args {
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			files = append(files, a)
			continue
		}
		entries, err := os.ReadDir(a)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") && supported(e.Name()) {
				files = append(files, filepath.Join(a, e.Name()))
			}
		}
	}
	return files, nil
}

func supported(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".pdf" || imageExts[ext]
}

func process(src string, cfg config) error {
	if !supported(src) {
		return fmt.Errorf("format neacceptat (doar PDF și jpg, png, tif, bmp, gif, webp)")
	}
	dir := cfg.out
	if dir == "" {
		dir = filepath.Dir(src)
	}
	dst := filepath.Join(dir, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))+".txt")

	tmp, err := os.MkdirTemp("", "webrt-ocr-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	var text, info string
	if strings.EqualFold(filepath.Ext(src), ".pdf") {
		text, info, err = ocrPDF(src, tmp, cfg)
	} else {
		text, err = ocrImage(src, tmp, cfg)
		info = "OCR"
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, []byte(text), 0o644); err != nil {
		return err
	}

	words := len(strings.Fields(text))
	fmt.Printf("%s → %s (%s, %d cuvinte)\n", filepath.Base(src), dst, info, words)
	if words == 0 {
		fmt.Println("  atenție: niciun text recunoscut (imagine goală, prea mică sau prea neclară?)")
	}
	return nil
}

// ---------- PDF ----------

// minTextChars: sub atâtea caractere (fără spații), pagina e considerată scanată.
const minTextChars = 20

func ocrPDF(src, tmp string, cfg config) (text, info string, err error) {
	n, err := pdfPages(src)
	if err != nil {
		return "", "", err
	}

	pages := make([]string, n)
	var ocrPages []int
	for p := 1; p <= n; p++ {
		if !cfg.force {
			if t, err := pdfPageText(src, p); err == nil && countNonSpace(t) >= minTextChars {
				pages[p-1] = t
				continue
			}
		}
		ocrPages = append(ocrPages, p)
	}

	errs := make([]error, n)
	parallel(ocrPages, cfg.workers, func(p int) {
		png := filepath.Join(tmp, fmt.Sprintf("p%05d", p))
		cmd := exec.Command("pdftoppm", "-r", strconv.Itoa(cfg.dpi), "-f", strconv.Itoa(p),
			"-l", strconv.Itoa(p), "-singlefile", "-png", src, png)
		if out, err := cmd.CombinedOutput(); err != nil {
			errs[p-1] = fmt.Errorf("pagina %d: pdftoppm: %s", p, lastLine(string(out), err))
			return
		}
		pages[p-1], errs[p-1] = tesseract(png+".png", tmp, cfg, fmt.Sprintf("pagina %d", p))
	})
	for _, e := range errs {
		if e != nil {
			return "", "", e
		}
	}

	var b strings.Builder
	for i, t := range pages {
		if cfg.marks {
			fmt.Fprintf(&b, "===== Pagina %d =====\n\n", i+1)
		}
		if t = strings.TrimSpace(t); t != "" {
			b.WriteString(t)
			b.WriteString("\n")
		}
		if i < len(pages)-1 {
			b.WriteString("\n")
		}
	}

	info = fmt.Sprintf("%d pagini", n)
	switch {
	case len(ocrPages) == n:
		info += ", toate prin OCR"
	case len(ocrPages) == 0:
		info += ", toate cu text digital, fără OCR"
	default:
		info += fmt.Sprintf(", %d prin OCR, %d cu text digital", len(ocrPages), n-len(ocrPages))
	}
	return b.String(), info, nil
}

var pagesRe = regexp.MustCompile(`(?m)^Pages:\s+(\d+)`)

func pdfPages(src string) (int, error) {
	out, err := exec.Command("pdfinfo", src).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo: %s", lastLine(string(out), err))
	}
	m := pagesRe.FindSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("nu pot citi numărul de pagini")
	}
	return strconv.Atoi(string(m[1]))
}

func pdfPageText(src string, p int) (string, error) {
	ps := strconv.Itoa(p)
	out, err := exec.Command("pdftotext", "-enc", "UTF-8", "-layout", "-f", ps, "-l", ps, src, "-").Output()
	return strings.TrimRight(string(out), "\f \n"), err
}

func countNonSpace(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// ---------- imagini ----------

// ocrImage: JPEG-urile rotite prin EXIF și formatele pe care tesseract nu le citește
// sigur (webp, gif) sunt normalizate întâi la PNG.
func ocrImage(src, tmp string, cfg config) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(src))
	input := src

	o := 1
	if ext == ".jpg" || ext == ".jpeg" {
		o = jpegOrientation(data)
	}
	if o != 1 || ext == ".webp" || ext == ".gif" {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("imagine ilizibilă: %w", err)
		}
		input = filepath.Join(tmp, "img.png")
		if err := writePNG(input, applyOrientation(img, o)); err != nil {
			return "", err
		}
	}

	t, err := tesseract(input, tmp, cfg, "")
	if err != nil {
		return "", err
	}
	if t = strings.TrimSpace(t); t != "" {
		t += "\n"
	}
	return t, nil
}

// ---------- tesseract ----------

// tesseract face OCR pe img; dacă pagina e întoarsă, o rotește întâi (într-o copie din tmp).
func tesseract(img, tmp string, cfg config, what string) (string, error) {
	if cfg.osd && !cfg.noRot {
		if rot := detectRotation(img); rot != 0 {
			rotated, err := rotateImage(img, tmp, rot)
			if err != nil {
				return "", fmt.Errorf("rotire %s: %w", what, err)
			}
			img = rotated
		}
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("tesseract", img, "stdout", "-l", cfg.lang, "--psm", strconv.Itoa(cfg.psm))
	// paralelismul îl facem noi pe pagini; tesseract multi-thread în paralel doar încetinește
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := lastLine(stderr.String(), err)
		if what != "" {
			msg = what + ": " + msg
		}
		return "", fmt.Errorf("tesseract: %s", msg)
	}
	text := strings.TrimRight(stdout.String(), "\f \n")
	if slices.Contains(strings.Split(cfg.lang, "+"), "ron") {
		text = romanianCommas.Replace(text)
	}
	return text, nil
}

// OCR-ul (și unele PDF-uri vechi) dau ş/ţ cu sedilă; forma corectă în română e cu virgulă.
var romanianCommas = strings.NewReplacer("ş", "ș", "ţ", "ț", "Ş", "Ș", "Ţ", "Ț")

var (
	osdRotate = regexp.MustCompile(`Rotate:\s*(\d+)`)
	osdConf   = regexp.MustCompile(`Orientation confidence:\s*([\d.]+)`)
)

// minOSDConfidence: sub acest prag detecția orientării e ghicit, deci nu rotim.
const minOSDConfidence = 2.0

// detectRotation întoarce câte grade (în sensul acelor de ceasornic) trebuie rotită
// imaginea: 0, 90, 180 sau 270. La orice eroare (ex. prea puțin text) → 0.
func detectRotation(img string) int {
	cmd := exec.Command("tesseract", img, "stdout", "--psm", "0", "-l", "osd")
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	r, c := osdRotate.FindSubmatch(out), osdConf.FindSubmatch(out)
	if r == nil || c == nil {
		return 0
	}
	if conf, _ := strconv.ParseFloat(string(c[1]), 64); conf < minOSDConfidence {
		return 0
	}
	rot, _ := strconv.Atoi(string(r[1]))
	return rot
}

func rotateImage(src, tmp string, deg int) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return "", err
	}
	// rotirea în sensul acelor de ceasornic, exprimată ca orientare EXIF
	o := map[int]int{90: 6, 180: 3, 270: 8}[deg]
	dst := filepath.Join(tmp, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))+".rot.png")
	return dst, writePNG(dst, applyOrientation(img, o))
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func parallel(items []int, workers int, fn func(int)) {
	sort.Ints(items)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < min(workers, len(items)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range jobs {
				fn(it)
			}
		}()
	}
	for _, it := range items {
		jobs <- it
	}
	close(jobs)
	wg.Wait()
}

// lastLine: uneltele scriu întâi avertismente, eroarea reală e la final.
func lastLine(s string, fallback error) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback.Error()
	}
	return s[strings.LastIndex(s, "\n")+1:]
}

// ---------- EXIF orientation ----------

func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xFF { // padding
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan / end of image
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2:]))
		if segLen < 2 || i+2+segLen > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+segLen]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + segLen
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}

func applyOrientation(src image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	s := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(s, s.Bounds(), src, b.Min, draw.Src)

	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	d := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			si, di := s.PixOffset(x, y), d.PixOffset(dx, dy)
			copy(d.Pix[di:di+4], s.Pix[si:si+4])
		}
	}
	return d
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Eroare: "+format+"\n", args...)
	os.Exit(1)
}
