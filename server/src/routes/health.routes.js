import express from 'express';
import embeddingService from '../services/embedding.service.js';
import qdrantService from '../services/qdrant.service.js';
import logger from '../utils/logger.js';

const router = express.Router();

/**
 * GET /api/health
 * Basic health check
 */
router.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    service: 'Search Server',
    timestamp: new Date().toISOString(),
    uptime: process.uptime(),
  });
});

/**
 * GET /api/health/detailed
 * Detailed health check including all services
 */
router.get('/health/detailed', async (req, res) => {
  try {
    const [embeddingHealth, qdrantHealth] = await Promise.all([
      embeddingService.healthCheck(),
      qdrantService.healthCheck(),
    ]);

    const allHealthy =
      embeddingHealth.status === 'healthy' &&
      qdrantHealth.status === 'healthy';

    res.status(allHealthy ? 200 : 503).json({
      status: allHealthy ? 'healthy' : 'degraded',
      timestamp: new Date().toISOString(),
      services: {
        embedding: embeddingHealth,
        qdrant: qdrantHealth,
      },
    });
  } catch (error) {
    logger.error('Health check error:', error);
    res.status(503).json({
      status: 'unhealthy',
      error: error.message,
      timestamp: new Date().toISOString(),
    });
  }
});

/**
 * GET /api/health/stats
 * Get database statistics
 */
router.get('/health/stats', async (req, res) => {
  try {
    const stats = await qdrantService.healthCheck();

    res.json({
      success: true,
      stats,
    });
  } catch (error) {
    logger.error('Stats endpoint error:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to retrieve stats',
      message: error.message,
    });
  }
});

/**
 * GET /api/stats
 * Get formatted database stats for UI dashboard
 */
router.get('/stats', async (req, res) => {
  try {
    const qdrantHealth = await qdrantService.healthCheck();

    if (qdrantHealth.status !== 'healthy') {
      throw new Error('Qdrant unavailable');
    }

    const pagesCount = qdrantHealth.collections.pages.count || 0;
    const pdfsCount = qdrantHealth.collections.pdfs.count || 0;
    const totalEntries = pagesCount + pdfsCount;

    // Format the entries count (e.g., 2800000 -> "2.8M+")
    const formatCount = (num) => {
      if (num >= 1000000) {
        return (num / 1000000).toFixed(1) + 'M+';
      } else if (num >= 1000) {
        return (num / 1000).toFixed(1) + 'K+';
      }
      return num.toString();
    };

    res.json({
      success: true,
      data: {
        entries: {
          value: formatCount(totalEntries),
          raw: totalEntries,
          label: 'Entries'
        },
        pages: {
          value: formatCount(pagesCount),
          raw: pagesCount,
          label: 'Web Pages'
        },
        pdfs: {
          value: formatCount(pdfsCount),
          raw: pdfsCount,
          label: 'PDF Documents'
        },
        available: {
          value: '',
          label: 'Available'
        }
      }
    });
  } catch (error) {
    logger.error('Stats endpoint error:', error);
    res.status(500).json({
      success: false,
      error: 'Failed to retrieve stats',
      message: error.message,
    });
  }
});

export default router;