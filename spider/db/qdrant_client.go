package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var Client *qdrant.Client

func InitQdrant() error {
	host := os.Getenv("QDRANT_HOST")
	port := 6334
	apiKey := os.Getenv("QDRANT_API_KEY")
	useTLS := false

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	Client, err = qdrant.NewClient(&qdrant.Config{
		Host:                   host,
		Port:                   port,
		APIKey:                 apiKey,
		UseTLS:                 useTLS,
		SkipCompatibilityCheck: true,
		GrpcOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		},
	})
	if err != nil {
		return fmt.Errorf("can't create Qdrant client: %w", err)
	}

	// Test connection
	_, err = Client.GetCollectionInfo(ctx, "test_connection")
	if err != nil && ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("Qdrant connection timeout, is server running at %s:%d?", host, port)
	}

	// Make sure collections exist
	if err := CreatePageEmbeddingsCollection(); err != nil {
		return fmt.Errorf("page embeddings collection creation failed: %w", err)
	}
	if err := CreatePDFEmbeddingsCollection(); err != nil {
		return fmt.Errorf("PDF embeddings collection creation failed: %w", err)
	}

	log.Println("Qdrant ready with page + PDF collections")
	return nil
}

// Check if a collection exists
func CheckCollectionExists(ctx context.Context, collectionName string) (bool, error) {
	info, err := Client.GetCollectionInfo(ctx, collectionName)
	if err != nil || info == nil {
		return false, nil
	}
	return true, nil
}

func CreatePageEmbeddingsCollection() error {
	exists, err := CheckCollectionExists(context.Background(), "page_content_embeddings")
	if err != nil {
		return fmt.Errorf("can't check page collection: %w", err)
	}
	if exists {
		return nil
	}

	err = Client.CreateCollection(context.Background(), &qdrant.CreateCollection{
		CollectionName: "page_content_embeddings",
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     384,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("can't create page collection: %w", err)
	}
	log.Println("Page embeddings collection created")
	return nil
}

func CreatePDFEmbeddingsCollection() error {
	exists, err := CheckCollectionExists(context.Background(), "pdf_content_embeddings")
	if err != nil {
		return fmt.Errorf("can't check PDF collection: %w", err)
	}
	if exists {
		log.Println("PDF collection already exists")
		return nil
	}

	err = Client.CreateCollection(context.Background(), &qdrant.CreateCollection{
		CollectionName: "pdf_content_embeddings",
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     384,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("can't create PDF collection: %w", err)
	}
	log.Println("PDF embeddings collection created")
	return nil
}

// Save a PDF embedding
func UpsertPDFEmbedding(pdfURL, pdfPath, pageURL string, embedding []float32, textChunk string, chunkIndex int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pointID := uuid.New().String()
	payload := map[string]interface{}{
		"pdf_url":     pdfURL,
		"pdf_path":    pdfPath,
		"page_url":    pageURL,
		"text_chunk":  textChunk,
		"chunk_index": chunkIndex,
		"timestamp":   time.Now().Unix(),
	}

	point := &qdrant.PointStruct{
		Id:      qdrant.NewID(pointID),
		Vectors: qdrant.NewVectors(embedding...),
		Payload: qdrant.NewValueMap(payload),
	}

	_, err := Client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: "pdf_content_embeddings",
		Points:         []*qdrant.PointStruct{point},
	})
	if err != nil {
		return fmt.Errorf("can't upsert PDF embedding: %w", err)
	}

	log.Printf("Stored PDF embedding %s chunk %d", pdfURL, chunkIndex)
	return nil
}

// Search PDF embeddings
func SearchPDFEmbeddings(queryEmbedding []float32, limit uint64) ([]*qdrant.ScoredPoint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := Client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: "pdf_content_embeddings",
		Query:          qdrant.NewQuery(queryEmbedding...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("PDF search failed: %w", err)
	}
	return result, nil
}

// Scroll PDFs by page URL
func SearchPDFsByPageURL(pageURL string, limit uint64) ([]*qdrant.ScoredPoint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("page_url", pageURL),
		},
	}
	scrollLimit := uint32(limit)

	scrollResult, err := Client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: "pdf_content_embeddings",
		Filter:         filter,
		Limit:          &scrollLimit,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	})
	if err != nil {
		return nil, fmt.Errorf("scroll PDF embeddings failed: %w", err)
	}

	points := make([]*qdrant.ScoredPoint, len(scrollResult))
	for i, point := range scrollResult {
		points[i] = &qdrant.ScoredPoint{
			Id:      point.Id,
			Payload: point.Payload,
			Score:   0,
		}
	}
	return points, nil
}

// Delete all embeddings of a PDF
func DeletePDFEmbeddings(pdfURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("pdf_url", pdfURL),
		},
	}

	_, err := Client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: "pdf_content_embeddings",
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: filter,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("can't delete PDF embeddings: %w", err)
	}
	log.Printf("Deleted all embeddings for PDF %s", pdfURL)
	return nil
}

// Hybrid search: pages + PDFs
func HybridSearch(queryEmbedding []float32, limit uint64) (map[string][]*qdrant.ScoredPoint, error) {
	results := make(map[string][]*qdrant.ScoredPoint)

	pageResults, err := SearchPageEmbeddings(queryEmbedding, limit)
	if err != nil {
		log.Printf("Warning: page search failed: %v", err)
	} else {
		results["pages"] = pageResults
	}

	pdfResults, err := SearchPDFEmbeddings(queryEmbedding, limit)
	if err != nil {
		log.Printf("Warning: PDF search failed: %v", err)
	} else {
		results["pdfs"] = pdfResults
	}

	return results, nil
}

// Search page embeddings
func SearchPageEmbeddings(queryEmbedding []float32, limit uint64) ([]*qdrant.ScoredPoint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := Client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: "page_content_embeddings",
		Query:          qdrant.NewQuery(queryEmbedding...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("page search failed: %w", err)
	}
	return result, nil
}

// Get PDF stats
func GetPDFEmbeddingsStats() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := Client.GetCollectionInfo(ctx, "pdf_content_embeddings")
	if err != nil {
		return nil, fmt.Errorf("can't get PDF collection info: %w", err)
	}

	stats := map[string]interface{}{
		"total_vectors":   info.GetPointsCount(),
		"vector_size":     info.GetConfig().GetParams().VectorsConfig.GetParams().GetSize(),
		"distance_metric": info.GetConfig().GetParams().VectorsConfig.GetParams().GetDistance().String(),
	}

	return stats, nil
}
