package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"podolyam/internal/demo"
	"podolyam/internal/money"
)

func rubles(value int64) string { return fmt.Sprintf("%d,%02d ₽", value/100, value%100) }

func main() {
	asJSON := flag.Bool("json", false, "Вывести входные данные и результат в JSON с копейками")
	flag.Parse()
	bill := demo.Restaurant()
	result, err := money.CalculateFinal(bill)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(struct {
			Bill   money.Bill   `json:"bill"`
			Result money.Result `json:"result"`
		}{bill, result}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	fmt.Println("Демонстрационный расчёт. Данные не сохраняются в БД.")
	fmt.Printf("Позиций: %d. Итого: %s\n", len(result.Lines), rubles(result.Total))
	names := map[string]string{}
	for _, p := range bill.Participants {
		names[p.ID] = p.Name
	}
	for _, total := range result.Totals {
		fmt.Printf("%s — %s; вернуть: %s\n", names[total.ParticipantID], rubles(total.Amount), rubles(*total.Debt))
	}
	fmt.Printf("К возврату плательщику: %s\n", rubles(*result.ToRepay))
}
