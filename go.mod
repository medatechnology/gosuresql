module github.com/medatechnology/gosuresql

go 1.27.1

require (
	github.com/joho/godotenv v1.5.1
	github.com/medatechnology/goutil v1.2.2
	github.com/medatechnology/simpleorm v0.2.0
	github.com/medatechnology/suresql v0.0.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.56.0 // indirect
)

// Local library checkouts (monorepo dev). Mirrors suresqlctl's wiring.
replace github.com/medatechnology/goutil => ../goutil

replace github.com/medatechnology/simpleorm => ../simpleorm

replace github.com/medatechnology/suresql => ../suresql
