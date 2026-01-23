import dotenv from 'dotenv';

dotenv.config();

export const config = {
  server: {
    port: parseInt(process.env.PORT || '3000', 10),
    env: process.env.NODE_ENV || 'development',
  },

  llama: {
    baseUrl: process.env.LLAMA_BASE_URL || 'http://llm-server:1410',
    model: process.env.LLAMA_MODEL || 'llama3.2',
    timeout: 60000,
  },

  embedding: {
    baseUrl: process.env.EMBEDDING_BASE_URL || 'http://embedding-service:5050',
    timeout: 30000,
  },

  qdrant: {
    host: process.env.QDRANT_HOST || 'qdrant',
    port: parseInt(process.env.QDRANT_PORT || '6333', 10),
    apiKey: process.env.QDRANT_API_KEY || undefined,
    collections: {
      pages: 'page_content_embeddings',
      pdfs: 'pdf_content_embeddings',
    },
  },

  search: {
    maxResults: parseInt(process.env.MAX_SEARCH_RESULTS || '15', 10),
    rerankTopK: parseInt(process.env.RERANK_TOP_K || '5', 10),
    chunkSize: parseInt(process.env.CHUNK_SIZE || '500', 10),
    chunkOverlap: parseInt(process.env.CHUNK_OVERLAP || '50', 10),
  },

  rateLimit: {
    windowMs: parseInt(process.env.RATE_LIMIT_WINDOW_MS || '60000', 10),
    maxRequests: parseInt(process.env.RATE_LIMIT_MAX_REQUESTS || '10000', 10),
  },
};

export default config;