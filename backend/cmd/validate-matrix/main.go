package main

import (
	"fmt"
	"os"

	"svoyshef/plugins/aidabaran"
)

const usage = `validate-matrix — валидатор данных рекомендателя (веха M1).

Проверяет data/$RESTAURANT/recommender/: matrix.json, heuristics.json,
upsell_map.json, forcart_priorities.json — против меню из data/$RESTAURANT/menu.json.

Отчёт построчно пишется в stdout: строки "ERROR ..." и "WARN ...".
Непокрытые блюдами пары печатаются как WARN всегда (и при покрытии < 80%,
и при >= 80%); на код возврата WARN не влияет. Финальные коды:
  exit 0  — ошибок нет (итоговая строка "OK: N pairs, coverage X%")
  exit 1  — есть хотя бы одна ERROR-строка (итог "FAILED: ...")
Фатальные ошибки (меню не читается) пишутся в stderr, exit 1.

env:
  DATA_DIR    корень данных ресторанов (по умолчанию "../data")
  RESTAURANT  подкаталог ресторана в DATA_DIR (по умолчанию "aidabaran")
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			fmt.Print(usage)
			return
		}
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "../data"
	}
	restaurant := os.Getenv("RESTAURANT")
	if restaurant == "" {
		restaurant = "aidabaran"
	}
	report, err := aidabaran.ValidateFiles(dataDir, restaurant)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v (data_dir=%s restaurant=%s)\n", err, dataDir, restaurant)
		os.Exit(1)
	}
	for _, f := range report.Findings {
		fmt.Printf("%s %s\n", f.Severity, f.Message)
	}
	if report.HasErrors() {
		fmt.Printf("FAILED: %d pairs, coverage %.1f%%, errors: %d\n",
			report.PairCount, report.Coverage, report.ErrorCount())
		os.Exit(1)
	}
	fmt.Printf("OK: %d pairs, coverage %.1f%%\n", report.PairCount, report.Coverage)
}
