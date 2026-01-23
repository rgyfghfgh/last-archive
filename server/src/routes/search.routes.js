import express from 'express';
import { body, validationResult } from 'express-validator';
import ragService from '../services/rag.service.js';
import logger from '../utils/logger.js';
import cors from 'cors';

const router = express.Router();
router.use(cors());

const validateSearchQuery = [
  body('query').trim().notEmpty().withMessage('Query cannot be empty'),
  body('sessionId').optional().isString().withMessage('Session ID must be a string')
];

/**
 * POST /api/search
 * Streaming endpoint using SSE (Server-Sent Events)
 */
router.post('/search', validateSearchQuery, async (req, res) => {
  const errors = validationResult(req);
  if (!errors.isEmpty()) {
    return res.status(400).json({ success: false, errors: errors.array() });
  }

  const { query, sessionId } = req.body;

  // Set headers for SSE
  res.setHeader('Content-Type', 'text/event-stream');
  res.setHeader('Cache-Control', 'no-cache');
  res.setHeader('Connection', 'keep-alive');

  const sendEvent = (type, data) => {
    if (typeof data !== 'string') {
      data = JSON.stringify(data);
    }
    if (type === 'token') {
      logger.debug(`Writing token to stream: "${data.substring(0, 30)}..."`);
    }
    res.write(`event: ${type}\n`);
    res.write(`data: ${data}\n\n`);
  };

  try {
    await ragService.searchStream(query, sessionId, (type, data) => {
      sendEvent(type, data);
    });
  } finally {
    res.end();
  }
});

// Helper for generic tests
router.get('/search/test', async (req, res) => {
  // We just return a message saying to use POST for streaming
  res.json({ message: "Please use POST /api/search for streaming results." });
});

export default router;