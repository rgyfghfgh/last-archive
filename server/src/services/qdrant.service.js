import { QdrantClient } from '@qdrant/js-client-rest';
import config from '../config/environment.js';
import logger from '../utils/logger.js';

class QdrantService {
  constructor() {
    this.client = new QdrantClient({
      url: `http://${config.qdrant.host}:${config.qdrant.port}`,
      apiKey: config.qdrant.apiKey,
    });

    this.collections = config.qdrant.collections;

    // min scores
    this.PDF_MIN_SIMILARITY = 0.55;
    this.PAGE_MIN_SIMILARITY = 0.5;
  }

  async searchPages(queryVector, limit = 10) {
    try {
      const results = await this.client.search(this.collections.pages, {
        vector: queryVector,
        limit,
        score_threshold: this.PAGE_MIN_SIMILARITY,
        with_payload: true,
      });

      logger.info(`Found ${results.length} pages`);
      return this.formatPageResults(results);
    } catch (error) {
      logger.error('Page search error:', error.message);
      throw new Error(`Page search failed: ${error.message}`);
    }
  }

  async searchPDFs(queryVector, limit = 10) {
    try {
      const results = await this.client.search(this.collections.pdfs, {
        vector: queryVector,
        limit,
        with_payload: true,
        score_threshold: this.PDF_MIN_SIMILARITY,
      });

      logger.info(`Found ${results.length} PDFs`);
      return this.formatPDFResults(results);
    } catch (error) {
      logger.error('PDF search error:', error.message);
      throw new Error(`PDF search failed: ${error.message}`);
    }
  }

  async hybridSearch(queryVector, limit = 15) {
    try {
      const [pageResults, pdfResults] = await Promise.all([
        this.searchPages(queryVector, limit),
        this.searchPDFs(queryVector, limit),
      ]);

      const filteredPDFs = this.filterPDFsBySimilarity(pdfResults);

      logger.info(`PDF check: ${pdfResults.length} -> ${filteredPDFs.length}`);

      return {
        pages: pageResults,
        pdfs: filteredPDFs,
        total: pageResults.length + filteredPDFs.length,
      };
    } catch (error) {
      logger.error('Search error:', error.message);
      throw error;
    }
  }

  // filter pdfs by score
  filterPDFsBySimilarity(pdfResults) {
    // Group PDFs by their PDF URL
    const pdfGroups = new Map();

    pdfResults.forEach(result => {
      const pdfUrl = result.pdfUrl;
      if (!pdfGroups.has(pdfUrl)) {
        pdfGroups.set(pdfUrl, []);
      }
      pdfGroups.get(pdfUrl).push(result);
    });

    // Filter: keep only PDFs where at least one chunk has score >= 0.7
    const filteredResults = [];

    pdfGroups.forEach((chunks, pdfUrl) => {
      const maxSimilarity = Math.max(...chunks.map(c => c.score));

      if (maxSimilarity >= this.PDF_MIN_SIMILARITY) {
        // Keep all chunks from this PDF (they'll be reranked later)
        filteredResults.push(...chunks);
        logger.debug(`PDF ${pdfUrl}: Max similarity ${maxSimilarity.toFixed(3)} - INCLUDED`);
      } else {
        logger.debug(`PDF ${pdfUrl}: Max similarity ${maxSimilarity.toFixed(3)} - EXCLUDED (< ${this.PDF_MIN_SIMILARITY})`);
      }
    });

    return filteredResults;
  }

  formatPageResults(results) {
    return results.map(result => ({
      id: result.id,
      score: result.score,
      url: result.payload?.url || '',
      title: result.payload?.title || '',
      content: result.payload?.main_content || '',
      metaDescription: result.payload?.meta_description || '',
      headings: result.payload?.headings || {},
      wordCount: result.payload?.word_count || 0,
      crawlDate: result.payload?.crawl_date || null,
    }));
  }

  formatPDFResults(results) {
    return results.map(result => ({
      id: result.id,
      score: result.score,
      pdfUrl: result.payload?.pdf_url || '',
      pageUrl: result.payload?.page_url || '',
      content: result.payload?.text_chunk || '',
      chunkIndex: result.payload?.chunk_index || 0,
      timestamp: result.payload?.timestamp || null,
    }));
  }

  async healthCheck() {
    try {
      const [pagesInfo, pdfsInfo] = await Promise.all([
        this.client.getCollection(this.collections.pages),
        this.client.getCollection(this.collections.pdfs),
      ]);

      return {
        status: 'healthy',
        collections: {
          pages: {
            name: this.collections.pages,
            count: pagesInfo.points_count,
          },
          pdfs: {
            name: this.collections.pdfs,
            count: pdfsInfo.points_count,
          },
        }
      };
    } catch (error) {
      logger.error('Health check failed:', error.message);
      return { status: 'error', error: error.message };
    }
  }
}

export default new QdrantService();