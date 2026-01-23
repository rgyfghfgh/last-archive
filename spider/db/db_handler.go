package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"spider/models"
	"spider/utils"

	_ "github.com/mattn/go-sqlite3"
	"github.com/qdrant/go-client/qdrant"
)

type SQLiteHandler struct {
	db           *sql.DB
	qdrantClient *qdrant.Client
}

var sqliteHandler *SQLiteHandler

func InitSQLite(dbPath string, qdrantClient *qdrant.Client) error {
	if dbPath == "" {
		dbPath = "./froxy.db"
	}

	log.Printf("Opening SQLite DB at: %s", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("can't open db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Println("Pinging DB to make sure it's alive...")
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("ping failed: %v", err)
	}
	log.Println("Connected to SQLite DB.")

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	sqliteHandler = &SQLiteHandler{
		db:           db,
		qdrantClient: qdrantClient,
	}

	if err := sqliteHandler.createTables(); err != nil {
		return fmt.Errorf("can't create tables: %v", err)
	}
	log.Println("DB tables and indexes checked/created.")

	return nil
}

func GetSQLiteHandler() *SQLiteHandler {
	return sqliteHandler
}

// helper for running stuff in a transaction
func (s *SQLiteHandler) withTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if err := s.HealthCheck(); err != nil {
		return fmt.Errorf("DB health check failed: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("can't begin transaction: %w", err)
	}

	var committed bool
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				if !strings.Contains(rbErr.Error(), "already been committed or rolled back") {
					log.Printf("Rollback failed: %v", rbErr)
				}
			}
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("can't commit transaction: %w", err)
	}
	committed = true
	return nil
}

func (s *SQLiteHandler) createTables() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	createPagesTable := `
	CREATE TABLE IF NOT EXISTS pages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		qdrant_id TEXT NOT NULL UNIQUE,
		url TEXT NOT NULL UNIQUE,
		title TEXT,
		status_code INTEGER,
		crawl_date DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	createImagesTable := `
	CREATE TABLE IF NOT EXISTS images (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		page_id INTEGER NOT NULL,
		image_url TEXT NOT NULL,
		image_path TEXT NOT NULL,
		alt_text TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE
	);`

	createIndexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_pages_url ON pages(url);",
		"CREATE INDEX IF NOT EXISTS idx_pages_qdrant_id ON pages(qdrant_id);",
		"CREATE INDEX IF NOT EXISTS idx_images_page_id ON images(page_id);",
		"CREATE INDEX IF NOT EXISTS idx_images_url ON images(image_url);",
	}

	tables := []string{createPagesTable, createImagesTable}

	for _, q := range tables {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("can't create table: %v", err)
		}
	}

	for _, q := range createIndexes {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("can't create index: %v", err)
		}
	}

	log.Println("Tables and indexes ready.")
	return nil
}

// batch insert images
func (s *SQLiteHandler) batchInsertImages(ctx context.Context, tx *sql.Tx, pageID int64, images []models.Image) error {
	if len(images) == 0 {
		return nil
	}

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO images (page_id, image_url, image_path, alt_text) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("can't prepare image insert: %w", err)
	}
	defer stmt.Close()

	for i, img := range images {
		if _, err := stmt.ExecContext(ctx, pageID, img.URL, img.Path, img.Alt); err != nil {
			return fmt.Errorf("can't insert image at %d: %w", i, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// Upsert page info
func (s *SQLiteHandler) UpsertPageData(pageData models.PageData) error {
	timeout := 120 * time.Second
	if len(pageData.Images) > 100 {
		timeout = 180 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	qdrantID := utils.GenerateUUIDFromURL(pageData.URL)
	var pageID int64

	err := s.withTransaction(ctx, func(tx *sql.Tx) error {
		upsertPageQuery := `
			INSERT INTO pages (qdrant_id, url, title, status_code, crawl_date, updated_at)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(url) DO UPDATE SET
				title = excluded.title,
				status_code = excluded.status_code,
				crawl_date = excluded.crawl_date,
				updated_at = CURRENT_TIMESTAMP
			RETURNING id;`

		err := tx.QueryRowContext(ctx, upsertPageQuery,
			qdrantID, pageData.URL, pageData.Title, pageData.StatusCode, pageData.CrawlDate,
		).Scan(&pageID)

		if err != nil {
			log.Printf("Can't upsert page %s: %v", pageData.URL, err)
			return fmt.Errorf("upsert page failed: %w", err)
		}

		// remove old images first
		if _, err := tx.ExecContext(ctx, "DELETE FROM images WHERE page_id = ?", pageID); err != nil {
			log.Printf("Can't delete old images for page %d: %v", pageID, err)
			return fmt.Errorf("delete images failed: %w", err)
		}

		if err := s.batchInsertImages(ctx, tx, pageID, pageData.Images); err != nil {
			log.Printf("Can't insert images for page %d: %v", pageID, err)
			return fmt.Errorf("insert images failed: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("upsert to SQLite failed: %w", err)
	}

	if err := UpsertPageToQdrant(s.qdrantClient, pageData); err != nil {
		log.Printf("Can't upsert page to Qdrant %s: %v", pageData.URL, err)
		return fmt.Errorf("upsert to Qdrant failed: %w", err)
	}

	log.Printf("Stored page %s (SQLite ID: %d, Qdrant ID: %s, Images: %d)", pageData.URL, pageID, qdrantID, len(pageData.Images))
	return nil
}

// get page data by URL
func (s *SQLiteHandler) GetPageByURL(url string) (*models.PageData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pageData := &models.PageData{Images: []models.Image{}}

	var pageID int64
	var qdrantID string
	query := `
		SELECT id, qdrant_id, url, title, status_code, crawl_date, updated_at
		FROM pages WHERE url = ?`

	err := s.db.QueryRowContext(ctx, query, url).Scan(
		&pageID, &qdrantID, &pageData.URL, &pageData.Title,
		&pageData.StatusCode, &pageData.CrawlDate, &pageData.LastModified,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get page failed: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, "SELECT image_url, image_path, alt_text FROM images WHERE page_id = ? ORDER BY id", pageID)
	if err != nil {
		return nil, fmt.Errorf("get images failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var img models.Image
		if err := rows.Scan(&img.URL, &img.Path, &img.Alt); err != nil {
			return nil, fmt.Errorf("scan image failed: %w", err)
		}
		pageData.Images = append(pageData.Images, img)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating images error: %w", err)
	}

	return pageData, nil
}

func (s *SQLiteHandler) GetPageCount() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pages").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("can't get page count: %w", err)
	}
	return count, nil
}

func (s *SQLiteHandler) HealthCheck() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("DB handler or connection is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	var tmp int
	if err := s.db.QueryRowContext(ctx, "SELECT 1").Scan(&tmp); err != nil {
		return fmt.Errorf("simple query test failed: %w", err)
	}

	return nil
}

func (s *SQLiteHandler) Close() error {
	if s.db != nil {
		log.Println("Closing DB connection...")
		return s.db.Close()
	}
	return nil
}

func (s *SQLiteHandler) GracefulShutdown(timeout time.Duration) error {
	if s.db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		log.Println("Shutting down DB...")
		done <- s.db.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("Error shutting down DB: %v", err)
		} else {
			log.Println("DB closed cleanly")
		}
		return err
	case <-ctx.Done():
		log.Println("DB shutdown timeout, forcing close")
		return fmt.Errorf("DB shutdown timeout")
	}
}
