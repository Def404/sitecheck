package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		urlFlag  = flag.String("url", "", "URL to check (repeatable)")
		file     = flag.String("f", "", "File with URL list (one per line)")
		interval = flag.Float64("i", 60, "Interval between checks, seconds")
		timeout  = flag.Float64("t", 10, "HTTP request timeout, seconds")
		once     = flag.Bool("once", false, "Check once, no loop")
		icmp     = flag.Bool("icmp", false, "ICMP ping (3 packets) in parallel with HTTP")
		logFile  = flag.String("log", "", "Save report to file (plain text) for support")
	)
	flag.Parse()

	var positional []string
	for _, a := range flag.Args() {
		positional = append(positional, a)
	}

	// Collect URLs from flags and positional args
	var rawURLs []string
	if *urlFlag != "" {
		rawURLs = append(rawURLs, *urlFlag)
	}
	rawURLs = append(rawURLs, positional...)

	if len(rawURLs) == 0 && *file == "" {
		fmt.Fprintln(os.Stderr, "Укажите URL(ы) или файл с -f/--file.")
		flag.Usage()
		os.Exit(1)
	}

	sites, err := loadSites(rawURLs, *file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка: %v\n", err)
		os.Exit(1)
	}
	if len(sites) == 0 {
		fmt.Fprintln(os.Stderr, "Нет ни одного URL для проверки.")
		os.Exit(1)
	}
	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "Интервал должен быть > 0.")
		os.Exit(1)
	}

	// Graceful shutdown
	stop := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		<-sigCh
		close(stop)
	}()

	// System info
	sysinfo := getSystemInfo()
	vpnName, vpnIPs := detectVPN()
	pingMode := ""
	if *icmp {
		pingMode = " + ICMP"
	}

	// Header
	fmt.Println()
	fmt.Printf("  %s⬢ Мониторинг сайтов%s\n", bold, reset)
	fmt.Printf("  %s%d сайт(ов)  ·  интервал %ds  ·  таймаут %ds%s%s\n", dim, len(sites), int(*interval), int(*timeout), pingMode, reset)
	fmt.Printf("  %s%s  ·  %s@%s  ·  %s%s\n", dim, sysinfo.OS, sysinfo.User, sysinfo.Host, sysinfo.IP, reset)
	if *icmp {
		fmt.Printf("  %sICMP-пинг: 3 пакета (дополнительная информация)%s\n", dim, reset)
	}
	fmt.Printf("  %s%s%s\n", dim, formatVPNStatus(vpnName, vpnIPs), reset)
	if *logFile != "" {
		fmt.Printf("  %sЛог: %s%s\n", dim, *logFile, reset)
	}
	fmt.Printf("  %sОстановка: Ctrl+C%s\n", dim, reset)
	fmt.Println()

	// Log file header
	if *logFile != "" {
		ts := time.Now().Format("2006-01-02 15:04:05")
		header := strings.Repeat("=", 70) + "\n" +
			fmt.Sprintf("Мониторинг сайтов  ·  запуск %s\n", ts) +
			strings.Repeat("=", 70) + "\n\n"
		appendToFile(*logFile, header)
	}

	prev := make(map[string]bool)
	roundNo := 0
	sessionStart := time.Now()
	interactive := !*once && isTTY()

	for {
		select {
		case <-stop:
			goto done
		default:
		}

		roundNo++
		results := runCheckRound(sites, *timeout, *icmp)
		table := renderRound(results, prev, roundNo, *icmp)
		dynamicPrint(table, interactive)

		if *logFile != "" {
			report := generateReport(results, roundNo, sysinfo, vpnName, vpnIPs, prev, sessionStart, *icmp)
			appendToFile(*logFile, report)
		}

		for _, r := range results {
			prev[r.URL] = r.OK
		}

		if *once {
			break
		}

		// Wait interval in small steps for responsive Ctrl+C
		waitLeft := time.Duration(*interval * float64(time.Second))
		for waitLeft > 0 {
			step := 500 * time.Millisecond
			if step > waitLeft {
				step = waitLeft
			}
			select {
			case <-stop:
				goto done
			case <-time.After(step):
			}
			waitLeft -= step
		}
	}

done:
	if interactive {
		clearScreen()
	}
	final := prev
	up, down := 0, 0
	for _, v := range final {
		if v {
			up++
		} else {
			down++
		}
	}
	upStr := green + fmt.Sprintf("%d доступно", up) + reset
	downStr := dim + fmt.Sprintf("%d недоступно", down) + reset
	if down > 0 {
		downStr = red + fmt.Sprintf("%d недоступно", down) + reset
	}
	fmt.Printf("\n  %s═══ Финал: %s, %s из %d ═══%s\n", bold, upStr, downStr, len(sites), reset)
}
