package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"podolyam/internal/local"
	"podolyam/internal/meetings"
	"podolyam/internal/money"
	"podolyam/internal/receipt"
)

// Сборочный скрипт помещает Vue-сборку сюда. В готовом exe она уже встроена.
//
//go:embed all:ui
var assets embed.FS

type App struct {
	ctx   context.Context
	store *local.Store
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }
func decode(body string, out any) error {
	if len(body) > 1<<20 {
		return errors.New("слишком большой документ")
	}
	d := json.NewDecoder(strings.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("некорректные поля документа")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("ожидается один документ")
	}
	return nil
}

func (a *App) prepareLocalDraft(ctx context.Context, draft meetings.Draft) (meetings.Draft, error) {
	profile, err := a.store.GetProfile(ctx)
	if err != nil {
		return draft, err
	}
	if strings.TrimSpace(profile.Name) == "" {
		return draft, errors.New("сначала укажите своё имя в разделе «Я»")
	}
	participants := []money.Participant{{ID: "me", Name: profile.Name, Order: 0}}
	for _, participant := range draft.Bill.Participants {
		if participant.ID == "me" {
			continue
		}
		participant.Order = int64(len(participants))
		participants = append(participants, participant)
	}
	draft.Bill.Participants = participants
	draft.Bill.PayerID = "me"
	return draft, nil
}

// Request — переходный адаптер для существующих экранов Vue.
// Это вызов Go через Wails, HTTP-сервер и сетевой порт не открываются.
func (a *App) Request(method, path, body string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	var out any = struct{}{}
	var err error
	if path == "/api/profile" {
		switch method {
		case "GET":
			out, err = a.store.GetProfile(ctx)
		case "PUT":
			var profile local.TransferProfile
			if err = decode(body, &profile); err == nil {
				out, err = a.store.SaveProfile(ctx, profile)
			}
		default:
			err = errors.New("неподдерживаемое действие")
		}
	} else if path == "/api/friends" {
		switch method {
		case "GET":
			out, err = a.store.ListFriends(ctx)
		case "POST":
			var friend local.Friend
			if err = decode(body, &friend); err == nil {
				out, err = a.store.SaveFriend(ctx, friend)
			}
		default:
			err = errors.New("неподдерживаемое действие")
		}
	} else if method == "DELETE" && strings.HasPrefix(path, "/api/friends/") {
		id := strings.TrimPrefix(path, "/api/friends/")
		if id == "" || strings.Contains(id, "/") {
			err = errors.New("неизвестное действие")
		} else {
			err = a.store.DeleteFriend(ctx, id)
		}
	} else if path == "/api/meetings" {
		switch method {
		case "GET":
			out, err = a.store.List(ctx)
		case "POST":
			var d meetings.Draft
			if err = decode(body, &d); err == nil {
				d, err = a.prepareLocalDraft(ctx, d)
			}
			if err == nil {
				out, err = a.store.Create(ctx, d)
			}
		default:
			err = errors.New("неподдерживаемое действие")
		}
	} else {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 3 || parts[0] != "api" || parts[1] != "meetings" {
			return "", errors.New("неизвестное действие")
		}
		id := parts[2]
		switch {
		case method == "GET" && len(parts) == 3:
			profile, profileErr := a.store.GetProfile(ctx)
			if profileErr != nil {
				err = profileErr
			} else if strings.TrimSpace(profile.Name) != "" {
				err = a.store.EnsureOrganizer(ctx, id, profile.Name)
			}
			if err == nil {
				out, err = a.store.Get(ctx, id)
			}
		case method == "DELETE" && len(parts) == 3:
			err = a.store.Delete(ctx, id)
		case method == "PUT" && len(parts) == 3:
			var d struct {
				meetings.Draft
				Version int64 `json:"version"`
			}
			if err = decode(body, &d); err == nil {
				err = a.store.Save(ctx, id, d.Version, d.Draft)
			}
		case method == "POST" && len(parts) == 4 && parts[3] == "finalize":
			var d struct {
				Version int64 `json:"version"`
			}
			if err = decode(body, &d); err == nil {
				err = a.store.Finalize(ctx, id, d.Version)
			}
		case method == "POST" && len(parts) == 4 && parts[3] == "payments":
			var d struct {
				Participant string `json:"participant_id"`
				Amount      int64  `json:"amount"`
			}
			if err = decode(body, &d); err == nil {
				err = a.store.Pay(ctx, id, d.Participant, d.Amount)
			}
		case method == "POST" && len(parts) == 6 && parts[3] == "payments" && parts[5] == "cancel":
			var d struct {
				Reason string `json:"reason"`
			}
			if err = decode(body, &d); err == nil {
				err = a.store.Cancel(ctx, id, parts[4], d.Reason)
			}
		case method == "POST" && len(parts) == 4 && parts[3] == "close":
			err = a.store.Finish(ctx, id)
		default:
			err = errors.New("это действие недоступно в локальном приложении")
		}
	}
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(out)
	return string(data), err
}
func (a *App) CopyText(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}
func safeFilename(value string) string {
	value = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return "расчёт"
	}
	return value
}

func receiptData(m meetings.Meeting, profile local.TransferProfile, participant string) (receipt.Data, error) {
	if m.State == "draft" {
		return receipt.Data{}, errors.New("сначала зафиксируйте расчёт")
	}
	result := receipt.Data{Meeting: m.Title, Date: m.Date, Venue: m.Venue, Phone: profile.Phone, Bank: profile.Bank, CreatedAt: time.Now(), Lines: []receipt.Line{}}
	for _, person := range m.Bill.Participants {
		if person.ID == participant {
			result.Participant = person.Name
		}
		if person.ID == m.Bill.PayerID {
			result.Payer = person.Name
		}
	}
	if result.Participant == "" {
		return receipt.Data{}, meetings.ErrNotFound
	}
	items := map[string]moneyItem{}
	for _, item := range m.Bill.Items {
		items[item.ID] = moneyItem{name: item.Name, rule: allocationRule(item, participant)}
	}
	for _, line := range m.Calculation.Lines {
		for _, share := range line.Shares {
			if share.ParticipantID == participant {
				item := items[line.ItemID]
				result.Lines = append(result.Lines, receipt.Line{Name: item.name, FullAmount: line.Amount, Rule: item.rule, Share: share.Amount})
			}
		}
	}
	for _, total := range m.Calculation.Totals {
		if total.ParticipantID == participant {
			result.Total = total.Amount
			if total.Debt != nil {
				result.Debt = *total.Debt
			}
		}
	}
	result.Remaining = m.Remaining[participant]
	result.Received = result.Debt - result.Remaining
	return result, nil
}

type moneyItem struct{ name, rule string }

func allocationRule(item money.Item, participant string) string {
	if item.Assignment == nil {
		return "не распределено"
	}
	totalWeight, ownWeight := int64(0), int64(0)
	for _, weight := range item.Assignment.Weights {
		totalWeight += weight.Value
		if weight.ParticipantID == participant {
			ownWeight = weight.Value
		}
	}
	switch item.Assignment.Mode {
	case money.Single:
		return "личная позиция"
	case money.Equal:
		return fmt.Sprintf("поровну: 1 из %d", len(item.Assignment.Weights))
	case money.All:
		return fmt.Sprintf("на всех: 1 из %d", len(item.Assignment.Weights))
	case money.Units:
		return fmt.Sprintf("единицы: %d из %d", ownWeight, totalWeight)
	case money.Weighted:
		return fmt.Sprintf("вес: %d из %d", ownWeight, totalWeight)
	default:
		return "неизвестное правило"
	}
}

func (a *App) ExportParticipantPDF(id, participant string) (string, error) {
	m, err := a.store.Get(a.ctx, id)
	if err != nil {
		return "", err
	}
	profile, err := a.store.GetProfile(a.ctx)
	if err != nil {
		return "", err
	}
	if profile.Phone == "" || profile.Bank == "" {
		return "", errors.New("сначала заполните телефон и банк в разделе «Я»")
	}
	data, err := receiptData(m, profile, participant)
	if err != nil {
		return "", err
	}
	windowsDir := os.Getenv("WINDIR")
	if windowsDir == "" {
		windowsDir = `C:\Windows`
	}
	pdf, err := receipt.Build(data, filepath.Join(windowsDir, "Fonts", "segoeui.ttf"), filepath.Join(windowsDir, "Fonts", "segoeuib.ttf"))
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Сохранить персональный чек", DefaultFilename: "PoDolyam-" + safeFilename(data.Participant) + ".pdf", Filters: []runtime.FileFilter{{DisplayName: "PDF", Pattern: "*.pdf"}}})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, pdf, 0600)
}
func main() {
	dir := os.Getenv("PODOLYAM_DATA_DIR")
	if dir == "" {
		dir = filepath.Join(os.Getenv("LOCALAPPDATA"), "PoDolyam")
	}
	if !filepath.IsAbs(dir) {
		panic("PODOLYAM_DATA_DIR должен быть абсолютным путём")
	}
	store, err := local.Open(filepath.Join(dir, "podolyam.db"))
	if err != nil {
		panic(err)
	}
	defer store.Close()
	app := &App{store: store}
	ui, err := fs.Sub(assets, "ui")
	if err != nil {
		panic(err)
	}
	err = wails.Run(&options.App{Title: "PoDolyam — разделим счёт", Width: 1120, Height: 820, MinWidth: 640, MinHeight: 600,
		AssetServer: &assetserver.Options{Assets: ui}, OnStartup: app.startup, Bind: []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "16ae03dd-7b4e-4c0a-baa4-0288660f69dc"},
		Windows:            &windows.Options{WebviewUserDataPath: filepath.Join(dir, "WebView2")},
	})
	if err != nil {
		panic(err)
	}
}
