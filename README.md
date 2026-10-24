# webrt-ocr

> Utilitar CLI în Go (1.26+), un singur binar.
> Recunoaște textul (OCR) din PDF-uri și imagini și îl scrie într-un `.txt` cu același nume.

```text
scan.pdf   → scan.txt
factura.jpg → factura.txt
```

Fișierele sursă nu sunt modificate niciodată.

## Cuprins

1. [Instalare / compilare](#1-instalare--compilare)
2. [Utilizare rapidă](#2-utilizare-rapidă)
3. [Opțiuni](#3-opțiuni)
4. [Cum funcționează](#4-cum-funcționează)
5. [Calitatea recunoașterii](#5-calitatea-recunoașterii)
6. [Comportament și limitări](#6-comportament-și-limitări)
7. [Dezvoltare](#7-dezvoltare)
8. [Referință rapidă fișiere](#8-referință-rapidă-fișiere)

## 1. Instalare / compilare

Cerință pentru compilare: Go 1.26 sau mai nou și `make`.

### Dependențe la rulare: tesseract + poppler-utils

OCR-ul este făcut de **tesseract**, iar paginile PDF sunt transformate în imagini cu
**poppler-utils** (`pdftoppm`, `pdfinfo`, `pdftotext`). Nu există un OCR bun scris doar în Go,
deci acestea trebuie instalate:

```bash
sudo apt install tesseract-ocr tesseract-ocr-ron poppler-utils
# macOS: brew install tesseract tesseract-lang poppler
```

`tesseract-ocr` aduce automat engleza (`eng`) și detecția orientării (`osd`). Pentru alte limbi:
`sudo apt install tesseract-ocr-deu` (germană), `-fra`, `-ita`, `-hun`...
Lista instalată: `tesseract --list-langs`.

Pentru imagini nu e nevoie de poppler-utils, doar pentru PDF-uri.

### Instalare globală (recomandat)

```bash
make install
```

Comanda compilează ca utilizatorul curent, apoi copiază binarul în `/usr/local/bin/webrt-ocr`.
Pentru copiere cere parola de `sudo`, fiindcă folderul aparține lui root. După aceea comanda
merge din orice folder.

Actualizare după modificări în cod: rulezi din nou `make install`.
Dezinstalare: `make uninstall`.

### Doar compilare locală

```bash
make build        # → ./webrt-ocr
make release      # → dist/ (Linux, macOS, Windows)
```

## 2. Utilizare rapidă

```bash
webrt-ocr scan.pdf                 # → scan.txt, lângă PDF
webrt-ocr poza1.jpg poza2.png      # → poza1.txt, poza2.txt
webrt-ocr .                        # toate PDF-urile și imaginile din folderul curent
webrt-ocr -o ~/texte *.pdf         # .txt-urile în alt folder
webrt-ocr -lang eng invoice.pdf    # doar engleză
webrt-ocr -pages -force carte.pdf  # OCR pe toate paginile, cu marcaj de pagină
```

Exemplu de ieșire:

```text
Statut.pdf → Statut.txt (15 pagini, toate prin OCR, 6452 cuvinte)
Oferta.pdf → Oferta.txt (3 pagini, toate cu text digital, fără OCR, 681 cuvinte)
bon.jpg → bon.txt (OCR, 84 cuvinte)
```

## 3. Opțiuni

| Opțiune     | Implicit      | Descriere |
|-------------|---------------|-----------|
| `-lang`     | `ron+eng`     | Limbile textului, cu `+` între ele. Mai puține limbi înseamnă un OCR mai rapid și mai precis. |
| `-o`        | lângă sursă   | Folderul în care se scriu `.txt`-urile. Se creează dacă nu există. |
| `-force`    | —             | OCR pe toate paginile PDF, și pe cele care au deja text digital. |
| `-pages`    | —             | Pune `===== Pagina N =====` la începutul fiecărei pagini PDF. |
| `-norotate` | —             | Dezactivează detecția și corectarea paginilor/pozelor întoarse. |
| `-dpi`      | `300`         | Rezoluția de randare a paginilor PDF. `400` ajută la scrisul mic; mai mult doar încetinește. |
| `-psm`      | `3`           | Modul de segmentare tesseract: `3` automat, `4` coloane/tabele, `6` un singur bloc de text, `11` text răzleț (bonuri, etichete). |
| `-j`        | nr. nuclee    | Câte pagini se procesează în paralel. |
| `-version`  | —             | Afișează versiunea. |

## 4. Cum funcționează

**PDF**

1. Pentru fiecare pagină se verifică dacă are deja text digital (PDF generat din Word etc.).
   Dacă da, textul este luat direct, exact, fără OCR. Paginile scanate merg la OCR.
   Un PDF amestecat (text + pagini scanate) este tratat corect pagină cu pagină.
2. Paginile de OCR sunt randate la 300 DPI și recunoscute în paralel, pe toate nucleele.
3. Paginile sunt puse în ordine în `.txt`, separate de un rând gol.

**Imagini** (`jpg`, `jpeg`, `png`, `tif`, `tiff`, `bmp`, `gif`, `webp`)

- Rotația EXIF a pozelor de telefon este aplicată înainte de OCR.
- WebP și GIF sunt convertite intern în PNG, pentru că tesseract nu le citește sigur.

**Pentru ambele**

- **Rotire automată:** o pagină scanată cu susul în jos sau pe lateral este detectată (tesseract OSD)
  și întoarsă înainte de OCR. Dacă detecția nu e sigură (prea puțin text), pagina rămâne cum e.
- **Diacritice:** când `-lang` conține `ron`, `ş ţ Ş Ţ` (cu sedilă) sunt înlocuite cu forma
  corectă `ș ț Ș Ț` (cu virgulă).

## 5. Calitatea recunoașterii

- Cel mai bine merg scanările drepte, la 300 DPI, cu text negru pe fundal alb.
- Pozele de telefon: fără umbre, cât mai drepte, cu textul cât mai mare în cadru.
- Scrisul de mână **nu** este recunoscut în mod util.
- Tabelele ies ca text pe rânduri, fără structură de coloane. Uneori `-psm 4` sau `-psm 6` dă un rezultat mai bun.
- Diacriticele de pe litere mari (`Ș`, `Ț`) sunt uneori citite fără virgulă (`S`, `T`).
  Pentru calitate maximă poți înlocui modelul standard cu cel „best”:

  ```bash
  sudo wget -O /usr/share/tesseract-ocr/5/tessdata/ron.traineddata \
    https://github.com/tesseract-ocr/tessdata_best/raw/main/ron.traineddata
  ```

  E mai lent, dar mai precis.

## 6. Comportament și limitări

- **Un `.txt` existent cu același nume este suprascris.**
- `poza.jpg` și `poza.pdf` din același folder scriu amândouă în `poza.txt`, deci rămâne doar ultimul.
- Un folder dat ca argument înseamnă doar fișierele din el, fără subfoldere și fără fișiere ascunse.
- Dacă un fișier eșuează (PDF corupt sau cu parolă, imagine ilizibilă), celelalte sunt procesate
  în continuare, iar codul de ieșire este 1.
- Dacă nu se recunoaște niciun cuvânt, `.txt`-ul este creat gol și apare un avertisment.
- Viteza orientativă: un scan de 15 pagini A4 durează ~20 s pe 12 nuclee (cu rotire automată).

## 7. Dezvoltare

```bash
make test     # go vet + go test
make build
```

Unelte externe apelate: `tesseract`, `pdfinfo`, `pdftotext`, `pdftoppm`.
Biblioteci Go: [`golang.org/x/image`](https://pkg.go.dev/golang.org/x/image) (WebP, BMP, TIFF).
Rotația EXIF este aceeași implementare ca în `webrt-img-resize`.

## 8. Referință rapidă fișiere

| Fișier         | Rol |
|----------------|-----|
| `main.go`      | Tot programul: flag-uri, PDF (text digital / OCR per pagină), imagini, rotire, tesseract. |
| `main_test.go` | Teste pentru formate, citirea folderelor, diacritice și rotire. |
| `Makefile`     | `build`, `install`, `uninstall`, `test`, `release`, `clean`. |
| `go.mod`       | Modulul și dependențele. |
