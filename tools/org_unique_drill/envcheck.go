//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"

	gomysql "github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
)

// Local drill-only DSN matches tools/org_unique_drill/start_mysql.ps1.
// Hardcoded here so shell commands never carry credentials.
const drillDSN = "drill:drill_only_local@tcp(127.0.0.1:13306)/peopleops_org_drill?charset=utf8mb4&parseTime=True&loc=Local"

func main() {
	dsn := strings.TrimSpace(os.Getenv("PEOPLEOPS_MYSQL_DRILL_DSN"))
	if dsn == "" {
		dsn = drillDSN
	}
	cfg, err := gomysql.ParseDSN(dsn)
	if err != nil {
		fmt.Println("PARSE_DSN_ERR")
		os.Exit(2)
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || host != "127.0.0.1" || port != "13306" || cfg.DBName != "peopleops_org_drill" {
		fmt.Println("UNSAFE_TARGET")
		fmt.Printf("host=%s port=%s db=%s net=%s\n", host, port, cfg.DBName, cfg.Net)
		os.Exit(3)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Println("OPEN_ERR")
		os.Exit(4)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Println("PING_ERR")
		os.Exit(5)
	}

	var version, currentDB string
	var tableCount int
	_ = db.QueryRow("SELECT VERSION()").Scan(&version)
	_ = db.QueryRow("SELECT DATABASE()").Scan(&currentDB)
	_ = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tableCount)

	fmt.Println("SAFE_TARGET_OK")
	fmt.Printf("mysql_version=%s\n", version)
	fmt.Printf("database=%s\n", currentDB)
	fmt.Printf("host=127.0.0.1 port=13306\n")
	fmt.Printf("table_count=%d\n", tableCount)
	fmt.Println("nature=ephemeral_local_drill_only")
	fmt.Println("destroyable=container peopleops-org-unique-drill OR drop database peopleops_org_drill")
}
