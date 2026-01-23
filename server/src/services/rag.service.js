import llamaService from './llama.service.js';
import embeddingService from './embedding.service.js';
import qdrantService from './qdrant.service.js';
import historyService from './history.service.js';
import config from '../config/environment.js';
import logger from '../utils/logger.js';

class Service {
  /**
   * Search and stream results
   * @param {string} userQuery
   * @param {string|null} sessionId
   * @param {Function} onEvent
   * Types: 'status', 'sources', 'token', 'error', 'done', 'session'
   */
  async searchStream(userQuery, sessionId, onEvent) {
    const startTime = Date.now();

    try {
      // 1. Start
      // 1. Setup
      onEvent('status', 'Loading...');

      // Handle Session
      let finalSessionId = sessionId;
      if (!finalSessionId) {
        finalSessionId = historyService.createSession();
      }
      onEvent('session', finalSessionId);

      logger.info(`Search: "${userQuery}" (${finalSessionId})`);

      const searchQuery = userQuery.trim();

      // History
      const history = historyService.getHistory(finalSessionId, 8);

      // Detect follow-up queries that don't need new search
      const isFollowUp = this.isFollowUpQuery(searchQuery);

      let searchResults;
      let enhancedQuery = searchQuery;

      if (isFollowUp && history.length > 0) {
        // For follow-up queries, use the last user query for search context
        const lastUserMessage = history.filter(h => h.role === 'user').pop();
        if (lastUserMessage) {
          enhancedQuery = lastUserMessage.content;
          logger.info(`Follow-up detected, using previous query: "${enhancedQuery}"`);
        }
      }

      // 2. Process
      onEvent('status', 'Searching...');
      const queryEmbeddingResult = await embeddingService.embed(enhancedQuery);

      // 3. Search
      onEvent('status', 'Searching...');
      searchResults = await qdrantService.hybridSearch(
        queryEmbeddingResult.embedding,
        config.search.maxResults
      );

      // 4. Filter
      onEvent('status', 'Filtering...');
      const allDocs = [
        ...searchResults.pages.map(p => ({ ...p, type: 'page' })),
        ...searchResults.pdfs.map(p => ({ ...p, type: 'pdf' }))
      ];

      const MIN_RELEVANCE_SCORE = 0.5;
      const topDocs = allDocs
        .filter(doc => doc.score >= MIN_RELEVANCE_SCORE)
        .sort((a, b) => b.score - a.score)
        .slice(0, config.search.rerankTopK);

      const finalDocs = topDocs.length > 0 ? topDocs : allDocs.sort((a, b) => b.score - a.score).slice(0, 3);

      // 6a. Capture Sources
      const sourcesData = {
        count: finalDocs.length,
        items: this.formatSources(finalDocs),
        searchTimeMs: Date.now() - startTime
      };

      // 5. Build
      onEvent('sources', sourcesData);
      onEvent('status', 'Processing...');
      const context = this.prepareOptimizedContext(finalDocs);

      // 6. Response
      let fullAnswer = '';
      await llamaService.generateStreamResponse(userQuery, context, history, (token) => {
        logger.debug(`Token: "${token.substring(0, 30)}..."`);
        fullAnswer += token;
        onEvent('token', token);
      });

      // Save to History
      if (fullAnswer.trim().length > 0) {
        historyService.addMessage(finalSessionId, 'user', userQuery);
        // Save assistant answer WITH sources
        historyService.addMessage(finalSessionId, 'assistant', fullAnswer, sourcesData);
      }

      // 7. Done
      onEvent('done', { totalTimeMs: Date.now() - startTime });
      logger.info(`Done in ${Date.now() - startTime}ms`);

    } catch (error) {
      logger.error('Error:', error);
      onEvent('error', error.message);
    }
  }

  // build context from docs
  prepareOptimizedContext(finalDocs) {
    const MAX_CONTEXT_LENGTH = 4000;
    const MAX_PER_DOC = 1000;

    let totalLength = 0;
    const selectedPages = [];
    const selectedPdfs = [];

    const sortedDocs = finalDocs.sort((a, b) => b.score - a.score);

    for (const doc of sortedDocs) {
      const content = doc.content || '';
      const truncatedContent = content.length > MAX_PER_DOC
        ? content.substring(0, MAX_PER_DOC) + '...'
        : content;

      if (totalLength + truncatedContent.length > MAX_CONTEXT_LENGTH) {
        break;
      }

      if (doc.type === 'page') {
        selectedPages.push({ ...doc, content: truncatedContent });
      } else {
        selectedPdfs.push({ ...doc, content: truncatedContent });
      }

      totalLength += truncatedContent.length;
    }

    return {
      pages: selectedPages,
      pdfs: selectedPdfs
    };
  }

  formatSources(docs) {
    return docs.map(doc => ({
      type: doc.type,
      url: doc.type === 'page' ? doc.url : doc.pdfUrl,
      title: doc.title || (doc.type === 'pdf' ? 'PDF Document' : 'Web Page'),
      score: doc.score
    }));
  }

  // check if user wants to continue previous chat
  isFollowUpQuery(query) {
    const normalized = query.toLowerCase().trim();

    // words that mean user wants more
    const followUpPatterns = [
      'continue',
      'go on',
      'tell me more',
      'more details',
      'elaborate',
      'explain more',
      'keep going',
      'and then',
      'what else',
      'anything else',
      'more about',
      'expand on',
      'can you continue',
      'please continue',
      'more information',
      'tell me about that',
      'what about',
      'how about'
    ];

    // check if user said any of these
    for (const pattern of followUpPatterns) {
      if (normalized === pattern || normalized.startsWith(pattern + ' ') || normalized.startsWith(pattern + '?')) {
        return true;
      }
    }

    // short questions probably mean continue
    const wordCount = normalized.split(/\s+/).length;
    if (wordCount <= 2 && normalized.length < 20) {
      return true;
    }

    return false;
  }
}

export default new Service();