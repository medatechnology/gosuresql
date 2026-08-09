module github.com/medatechnology/gosuresql

go 1.23.2

require (
	github.com/joho/godotenv v1.5.1
	github.com/medatechnology/goutil v1.2.2
	github.com/medatechnology/simpleorm v0.2.0
	github.com/medatechnology/suresql v0.0.1
)

require github.com/lib/pq v1.10.9 // indirect

// Local library checkouts (monorepo dev). Mirrors suresqlctl's wiring.
replace github.com/medatechnology/goutil => ../goutil

replace github.com/medatechnology/simpleorm => ../simpleorm

replace github.com/medatechnology/suresql => ../suresql
