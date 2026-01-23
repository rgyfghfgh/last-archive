import express from 'express';
import historyService from '../services/history.service.js';
import logger from '../utils/logger.js';
import cors from 'cors';

const router = express.Router();
router.use(cors());

/**
 * GET /api/sessions
 * List all chat sessions
 */
router.get('/sessions', (req, res) => {
    try {
        const sessions = historyService.getAllSessions();
        res.json({ success: true, sessions });
    } catch (error) {
        logger.error('Error fetching sessions:', error);
        res.status(500).json({ success: false, error: error.message });
    }
});

/**
 * GET /api/sessions/:id
 * Get full history for a specific session
 */
router.get('/sessions/:id', (req, res) => {
    try {
        const { id } = req.params;
        const history = historyService.getFullHistory(id);
        const session = historyService.getSession(id);

        if (!session) {
            return res.status(404).json({ success: false, error: 'Session not found' });
        }

        res.json({ success: true, session, history });
    } catch (error) {
        logger.error('Error fetching session details:', error);
        res.status(500).json({ success: false, error: error.message });
    }
});

/**
 * DELETE /api/sessions/:id
 * Delete a session
 */
router.delete('/sessions/:id', (req, res) => {
    try {
        const { id } = req.params;
        const deleted = historyService.deleteSession(id);

        if (!deleted) {
            return res.status(404).json({ success: false, error: 'Session not found' });
        }

        res.json({ success: true, message: 'Session deleted' });
    } catch (error) {
        logger.error('Error deleting session:', error);
        res.status(500).json({ success: false, error: error.message });
    }
});

/**
 * PATCH /api/sessions/:id
 * Update session title
 */
router.patch('/sessions/:id', (req, res) => {
    try {
        const { id } = req.params;
        const { title } = req.body;

        if (!title) {
            return res.status(400).json({ success: false, error: 'Title is required' });
        }

        const updated = historyService.updateSessionTitle(id, title);

        if (!updated) {
            return res.status(404).json({ success: false, error: 'Session not found' });
        }

        res.json({ success: true, message: 'Session updated' });
    } catch (error) {
        logger.error('Error updating session:', error);
        res.status(500).json({ success: false, error: error.message });
    }
});

export default router;
