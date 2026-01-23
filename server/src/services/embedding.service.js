import axios from 'axios';
import config from '../config/environment.js';
import logger from '../utils/logger.js';

class EmbeddingService {
  constructor() {
    this.client = axios.create({
      baseURL: config.embedding.baseUrl,
      timeout: config.embedding.timeout,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // cache for speed
    this.cache = new Map();
    this.cacheSize = 500;
  }

  async embed(text) {
    try {
      if (!text || !text.trim()) {
        throw new Error('Text cannot be empty');
      }

      const trimmedText = text.trim();

      // Cache check
      if (this.cache.has(trimmedText)) {
        logger.debug('Using cache');
        return this.cache.get(trimmedText);
      }

      const response = await this.client.post('/embed', { text: trimmedText });

      const result = {
        embedding: response.data.embedding,
        dims: response.data.dims,
        elapsedMs: response.data.elapsed_ms,
      };

      // Cache the result (with size limit)
      if (this.cache.size >= this.cacheSize) {
        const firstKey = this.cache.keys().next().value;
        this.cache.delete(firstKey);
      }
      this.cache.set(trimmedText, result);

      return result;
    } catch (error) {
      logger.error('Service error:', error.message);
      throw new Error(`Failed: ${error.message}`);
    }
  }

  async embedBatch(texts) {
    try {
      const embeddings = await Promise.all(
        texts.map(text => this.embed(text))
      );

      return embeddings;
    } catch (error) {
      logger.error('Batch embedding error:', error.message);
      throw error;
    }
  }

  async healthCheck() {
    try {
      const response = await this.client.get('/health');
      return response.data;
    } catch (error) {
      logger.error('Embedding service health check failed:', error.message);
      return { status: 'unhealthy', error: error.message };
    }
  }
}

export default new EmbeddingService();