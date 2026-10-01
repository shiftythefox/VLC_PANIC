# VLC ESC Smiley

Pici Windows-segedprogram VLC-hez.

## Mit csinal?

- Egy **20x20 pixeles smiley** jelenik meg, keret nelkul.
- Mindig a tobbi ablak felett marad.
- **Bal egergombbal huzhato** a kepernyon.
- **Jobb klikk a smiley-n: kilepes.**
- Amig fut, globalisan figyeli az **ESC** billentyut.
- Ha ESC lenyomasakor fut egy lathato `vlc.exe` ablak:
  1. VLC kap egy dedikalt **MEDIA_PAUSE** parancsot;
  2. VLC minimalizalodik a talcara;
  3. az adott ESC lenyomast nem kapja meg a VLC.
- Ha VLC nem fut, az ESC normalisan mukodik.
- Egyszerre csak egy peldany indul el.

## Kesz EXE

A kiadott `VLC_ESC_Smiley.exe` onallo Windows x64 program, **Python nem kell hozza**.

## Forditas forrasbol

### Windows

1. Telepitsd a Go-t: https://go.dev/dl/
2. Futtasd a `build.bat` fajlt.
3. Az eredmeny: `release\VLC_ESC_Smiley.exe`

Vagy parancssorbol:

```bat
set GOOS=windows
set GOARCH=amd64
go build -buildvcs=false -trimpath -ldflags="-s -w -H=windowsgui" -o release\VLC_ESC_Smiley.exe .\src
```

## Projekt szerkezete

```text
VLC_ESC_Smiley_Git/
├─ src/
│  └─ main.go                  # natív Windows/Go valtozat
├─ python/
│  └─ VLC_ESC_Smiley.py        # eredeti Python valtozat
├─ release/
│  └─ VLC_ESC_Smiley.exe       # kesz x64 build
├─ .github/workflows/
│  └─ build-windows.yml        # GitHub Actions build
├─ .gitignore
├─ build.bat
├─ go.mod
└─ README.md
```

## Megjegyzes

A natív Go valtozat csak Windowsra keszult. A globalis billentyuzetfigyeleshez nem telepit drivert es nem hasznal kulso csomagot; a Windows sajat low-level keyboard hook API-jat hasznalja.
