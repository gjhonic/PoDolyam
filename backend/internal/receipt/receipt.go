// Package receipt формирует локальный персональный чек из зафиксированного расчёта.
package receipt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/signintech/gopdf"
)

type Line struct {
	Name       string
	FullAmount int64
	Rule       string
	Share      int64
}

type Data struct {
	Meeting, Date, Venue, Participant, Payer string
	Phone, Bank                              string
	Total, Debt, Received, Remaining         int64
	Lines                                    []Line
	CreatedAt                                time.Time
}

func money(value int64) string {
	sign := ""
	if value < 0 {
		sign, value = "-", -value
	}
	return fmt.Sprintf("%s%d,%02d руб.", sign, value/100, value%100)
}

// Build возвращает готовый PDF. Шрифты передаются путями, чтобы использовать
// штатные Segoe UI из Windows и не хранить лицензированные TTF в репозитории.
func Build(data Data, regularFont, boldFont string) ([]byte, error) {
	if strings.TrimSpace(data.Participant) == "" || strings.TrimSpace(data.Meeting) == "" {
		return nil, errors.New("для чека нужны встреча и участник")
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFont("regular", regularFont); err != nil {
		return nil, fmt.Errorf("шрифт Windows: %w", err)
	}
	if err := pdf.AddTTFFont("bold", boldFont); err != nil {
		return nil, fmt.Errorf("жирный шрифт Windows: %w", err)
	}

	const left, right, top, bottom = 44.0, 551.0, 44.0, 797.0
	y := top
	page := 0
	addPage := func() error {
		pdf.AddPage()
		page++
		y = top
		pdf.SetFillColor(21, 21, 21)
		pdf.RectFromUpperLeftWithStyle(0, 0, 595.28, 22, "F")
		if err := pdf.SetFont("regular", "", 8); err != nil {
			return err
		}
		pdf.SetTextColor(90, 90, 90)
		pdf.SetXY(left, 812)
		return pdf.CellWithOption(&gopdf.Rect{W: right - left, H: 12}, fmt.Sprintf("PoDolyam - персональный чек · стр. %d", page), gopdf.CellOption{Align: gopdf.Right})
	}
	if err := addPage(); err != nil {
		return nil, err
	}
	ensure := func(height float64) error {
		if y+height <= bottom {
			return nil
		}
		return addPage()
	}
	write := func(text, font string, size, height float64, color [3]uint8) error {
		if err := ensure(height); err != nil {
			return err
		}
		if err := pdf.SetFont(font, "", size); err != nil {
			return err
		}
		pdf.SetTextColor(color[0], color[1], color[2])
		pdf.SetXY(left, y)
		if err := pdf.CellWithOption(&gopdf.Rect{W: right - left, H: height}, text, gopdf.CellOption{Align: gopdf.Left | gopdf.Middle}); err != nil {
			return err
		}
		y += height
		return nil
	}
	black, gray, pink := [3]uint8{21, 21, 21}, [3]uint8{84, 84, 84}, [3]uint8{210, 35, 116}
	if err := write("PODOLYAM", "bold", 12, 18, pink); err != nil {
		return nil, err
	}
	if err := write(data.Meeting, "bold", 25, 34, black); err != nil {
		return nil, err
	}
	meta := data.Date
	if strings.TrimSpace(data.Venue) != "" {
		meta += " · " + data.Venue
	}
	if err := write(meta, "regular", 10, 20, gray); err != nil {
		return nil, err
	}
	if err := write("ПЕРСОНАЛЬНЫЙ РАСЧЁТ: "+data.Participant, "bold", 14, 30, black); err != nil {
		return nil, err
	}

	if err := pdf.SetFont("bold", "", 9); err != nil {
		return nil, err
	}
	pdf.SetFillColor(234, 255, 53)
	pdf.RectFromUpperLeftWithStyle(left, y, right-left, 24, "F")
	pdf.SetTextColor(black[0], black[1], black[2])
	pdf.SetXY(left+6, y)
	if err := pdf.CellWithOption(&gopdf.Rect{W: 270, H: 24}, "Что вы ели", gopdf.CellOption{Align: gopdf.Left | gopdf.Middle}); err != nil {
		return nil, err
	}
	pdf.SetXY(left+282, y)
	if err := pdf.CellWithOption(&gopdf.Rect{W: 95, H: 24}, "Расчёт", gopdf.CellOption{Align: gopdf.Left | gopdf.Middle}); err != nil {
		return nil, err
	}
	pdf.SetXY(left+383, y)
	if err := pdf.CellWithOption(&gopdf.Rect{W: 118, H: 24}, "Ваша доля", gopdf.CellOption{Align: gopdf.Right | gopdf.Middle}); err != nil {
		return nil, err
	}
	y += 28
	for i, line := range data.Lines {
		if err := ensure(37); err != nil {
			return nil, err
		}
		if err := pdf.SetFont("regular", "", 9); err != nil {
			return nil, err
		}
		pdf.SetTextColor(black[0], black[1], black[2])
		pdf.SetXY(left+6, y)
		label := fmt.Sprintf("%02d  %s", i+1, line.Name)
		if err := pdf.CellWithOption(&gopdf.Rect{W: 270, H: 18}, label, gopdf.CellOption{Align: gopdf.Left | gopdf.Middle}); err != nil {
			return nil, err
		}
		pdf.SetXY(left+282, y)
		if err := pdf.CellWithOption(&gopdf.Rect{W: 95, H: 18}, line.Rule, gopdf.CellOption{Align: gopdf.Left | gopdf.Middle}); err != nil {
			return nil, err
		}
		pdf.SetXY(left+383, y)
		if err := pdf.CellWithOption(&gopdf.Rect{W: 118, H: 18}, money(line.Share), gopdf.CellOption{Align: gopdf.Right | gopdf.Middle}); err != nil {
			return nil, err
		}
		pdf.SetTextColor(gray[0], gray[1], gray[2])
		pdf.SetXY(left+30, y+17)
		if err := pdf.CellWithOption(&gopdf.Rect{W: 245, H: 13}, "Полная стоимость: "+money(line.FullAmount), gopdf.CellOption{Align: gopdf.Left}); err != nil {
			return nil, err
		}
		pdf.SetStrokeColor(190, 190, 190)
		pdf.Line(left, y+33, right, y+33)
		y += 37
	}
	if len(data.Lines) == 0 {
		if err := write("В расчёте нет назначенных позиций.", "regular", 10, 28, gray); err != nil {
			return nil, err
		}
	}

	y += 8
	if err := ensure(116); err != nil {
		return nil, err
	}
	pdf.SetFillColor(21, 21, 21)
	pdf.RectFromUpperLeftWithStyle(left, y, right-left, 108, "F")
	rows := []struct {
		label  string
		value  int64
		bright bool
	}{
		{"Ваша доля в чеке", data.Total, false}, {"Обязательство перед плательщиком", data.Debt, false},
		{"Уже переведено", data.Received, false}, {"Осталось перевести", data.Remaining, true},
	}
	for _, row := range rows {
		if err := pdf.SetFont("regular", "", 10); err != nil {
			return nil, err
		}
		pdf.SetTextColor(255, 255, 255)
		pdf.SetXY(left+14, y+8)
		if err := pdf.CellWithOption(&gopdf.Rect{W: 310, H: 20}, row.label, gopdf.CellOption{Align: gopdf.Left | gopdf.Middle}); err != nil {
			return nil, err
		}
		if err := pdf.SetFont("bold", "", 12); err != nil {
			return nil, err
		}
		if row.bright {
			pdf.SetTextColor(234, 255, 53)
		} else {
			pdf.SetTextColor(255, 255, 255)
		}
		pdf.SetXY(left+330, y+8)
		if err := pdf.CellWithOption(&gopdf.Rect{W: 162, H: 20}, money(row.value), gopdf.CellOption{Align: gopdf.Right | gopdf.Middle}); err != nil {
			return nil, err
		}
		y += 24
	}
	y += 22
	if err := write("КУДА ПЕРЕВЕСТИ", "bold", 13, 25, pink); err != nil {
		return nil, err
	}
	transfer := "Плательщик: " + data.Payer + "\nТелефон: " + data.Phone + "\nБанк: " + data.Bank
	if err := ensure(70); err != nil {
		return nil, err
	}
	if err := pdf.SetFont("regular", "", 11); err != nil {
		return nil, err
	}
	pdf.SetTextColor(black[0], black[1], black[2])
	pdf.SetXY(left, y)
	if err := pdf.MultiCellWithOption(&gopdf.Rect{W: right - left, H: 66}, transfer, gopdf.CellOption{Align: gopdf.Left, CoefLineHeight: 1.5}); err != nil {
		return nil, err
	}
	y += 70
	created := data.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	if err := write("Снимок сформирован "+created.Format("02.01.2006 15:04")+". Последующие переводы в этом файле не обновляются.", "regular", 8, 20, gray); err != nil {
		return nil, err
	}
	return pdf.GetBytesPdf(), nil
}
