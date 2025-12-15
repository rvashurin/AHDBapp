# AHDBapp
The out of game parser, processor, uploader for AHDB wow addon

WIP, works with https://github.com/mooreatv/AuctionDB

Already includes a pretty cool
- [lua2json.sh](lua2json.sh) Lua (tables/WoW saved variables) to JSON converter
- [lua2json golang package](lua2json/) Golang version of the sed+awk script hack


## Getting started

ahdb.go now reads lua and writes to a MySql DB directly (needed schema is in schema.sql)

Environment variables to control the access to the DB:
- optional `MYSQL_USER` (defaults to root)
- `MYSQL_PASSWORD`
- optional `MYSQL_CONNECTION_INFO` (defaults to tcp to 3306)

You need:
- golang https://golang.org/dl/ (on windows you may still need to get git (https://git-scm.com/downloads)
- then type `go install github.com/mooreatv/AHDBapp@latest` it will download and build and install the binary in ~/go/bin
- then
  - On windows `go\bin\AHDBapp.exe < "c:\Program Files (x86)\World of Warcraft\_classic_era_\WTF\Account\YOURACCOUNT\SavedVariables\AuctionDB.lua"`
  - On unix/mac `~/go/bin/AHDBapp < ...path_to_.../SavedVariables/AuctionDB.lua`

## PoC web app (graphs)

This repo now also includes a small local web UI to graph item prices over time (mean/median + stddev per scan snapshot).

Run it:
- `MYSQL_PASSWORD=... go run ./cmd/ahdbweb`
- open `http://127.0.0.1:8080` in your browser

Optional env vars (same as the importer):
- `MYSQL_USER` (defaults to `root`)
- `MYSQL_CONNECTION_INFO` (defaults to `tcp(:3306)`)

Optional flag:
- `-addr 127.0.0.1:8080` (change listen address/port)

### old instructions
You used to need/do
- golang https://golang.org/dl/
- then type `go get github.com/mooreatv/AHDBapp` it will download the source into `~/go/src/github/mooreatv/AHDBapp` (and build the binary in ~/go/bin)
- then
  - On windows `go\bin\AHDBapp.exe < "c:\Program Files (x86)\World of Warcraft\_classic_\WTF\Account\YOURACCOUNT\SavedVariables\AuctionDB.lua" > data.csv`
  - On unix/mac `~/go/bin/AHDBapp < ...path_to_.../SavedVariables/AuctionDB.lua > data.csv`

### even older instructions
You used to need
- bash and some basic unix utilities; easiest way to get those is through git bash that comes with https://git-scm.com/downloads
- golang https://golang.org/dl/
- `go get github.com/mooreatv/AHDBapp` it will download into `~/go/src/github/mooreatv/AHDBapp`
- ./ahdbSavedVars2Json.sh YOURWOWACCOUNT
- go run ahdb.go < auctiondb.json > auctiondb.csv

But now you just need golang as `go run ahdb.go` can process the saved variables directly as above
