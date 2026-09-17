package receipt

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func testFonts(t *testing.T) (string, string) {
	t.Helper()
	pairs := [][2]string{
		{filepath.Join(os.Getenv("WINDIR"), "Fonts", "segoeui.ttf"), filepath.Join(os.Getenv("WINDIR"), "Fonts", "segoeuib.ttf")},
		{"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"},
	}
	for _, pair := range pairs {
		if _, err := os.Stat(pair[0]); err == nil {
			if _, err = os.Stat(pair[1]); err == nil {
				return pair[0], pair[1]
			}
		}
	}
	t.Skipf("Unicode TTF не найден для %s", runtime.GOOS)
	return "", ""
}

func TestBuildPersonalReceipt(t *testing.T) {
	regular, bold := testFonts(t)
	data := Data{
		Meeting: "Ужин после релиза", Date: "17.09.2026", Venue: "Нэко", Participant: "Дима", Payer: "Женя",
		Phone: "+79991234567", Bank: "Т-Банк", Total: 152168, Debt: 152168, Received: 50000, Remaining: 102168,
		CreatedAt: time.Date(2026, 9, 17, 14, 30, 0, 0, time.Local),
		Lines:     []Line{{Name: "Рамен со свининой", FullAmount: 46000, Rule: "одному", Share: 46000}, {Name: "Гавайская пицца", FullAmount: 68000, Rule: "на всех", Share: 11334}, {Name: "Нэко сет", FullAmount: 99000, Rule: "на всех", Share: 16500}},
	}
	pdf, err := Build(data, regular, bold)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) < 5_000 || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("некорректный PDF: %d байт", len(pdf))
	}
	if output := os.Getenv("PODOLYAM_PDF_PREVIEW"); output != "" {
		if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, pdf, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildRequiresParticipant(t *testing.T) {
	regular, bold := testFonts(t)
	if _, err := Build(Data{Meeting: "Ужин"}, regular, bold); err == nil {
		t.Fatal("создан безымянный чек")
	}
}

func TestFormatPhone(t *testing.T) {
	tests := map[string]string{
		"+79247595040":      "+7 924 759-50-40",
		"8 (999) 123-45-67": "+7 999 123-45-67",
		"12345":             "12345",
		"   ":               "не указано",
	}
	for input, want := range tests {
		if got := formatPhone(input); got != want {
			t.Errorf("formatPhone(%q) = %q, want %q", input, got, want)
		}
	}
}
