import Database from 'better-sqlite3';
import { v4 as uuidv4 } from 'uuid';
import logger from '../utils/logger.js';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const dataDir = path.join(__dirname, '../../data');

// Ensure data directory exists
if (!fs.existsSync(dataDir)) {
    fs.mkdirSync(dataDir, { recursive: true });
}

class HistoryService {
    constructor() {
        const dbPath = path.join(dataDir, 'chat.db');
        this.db = new Database(dbPath);
        this.init();
    }

    init() {
        try {
            // Create tables if not exist
            this.db.exec(`
        CREATE TABLE IF NOT EXISTS sessions (
          id TEXT PRIMARY KEY,
          title TEXT,
          created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
          updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS messages (
          id INTEGER PRIMARY KEY AUTOINCREMENT,
          session_id TEXT NOT NULL,
          role TEXT NOT NULL,
          content TEXT NOT NULL,
          created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
          sources TEXT,
          FOREIGN KEY (session_id) REFERENCES sessions (id) ON DELETE CASCADE
        );

        CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
      `);
            logger.info('Database ready');
        } catch (error) {
            logger.error('Database error:', error);
        }
    }

    createSession(title = 'New Chat') {
        const id = uuidv4();
        try {
            const stmt = this.db.prepare('INSERT INTO sessions (id, title) VALUES (?, ?)');
            stmt.run(id, title);
            return id;
        } catch (error) {
            logger.error('Failed to create session:', error);
            throw error;
        }
    }

    getSession(id) {
        try {
            const stmt = this.db.prepare('SELECT * FROM sessions WHERE id = ?');
            return stmt.get(id);
        } catch (error) {
            return null;
        }
    }

    addMessage(sessionId, role, content, sources = null) {
        try {
            // Ensure session exists
            const session = this.getSession(sessionId);
            if (!session) {
                this.createSession('Auto-created Session');
            }

            // Update title if it's the first user message and title is default
            if (role === 'user') {
                const currentSession = this.getSession(sessionId);
                if (currentSession && (currentSession.title === 'New Chat' || currentSession.title === 'Auto-created Session')) {
                    // Truncate to 50 chars and remove newlines
                    const newTitle = content.split('\n')[0].substring(0, 50).trim();
                    this.updateSessionTitle(sessionId, newTitle || 'New Chat');
                }
            }

            const stmt = this.db.prepare('INSERT INTO messages (session_id, role, content, sources) VALUES (?, ?, ?, ?)');
            const sourcesJson = sources ? JSON.stringify(sources) : null;
            stmt.run(sessionId, role, content, sourcesJson);

            // Update session timestamp
            const updateStmt = this.db.prepare('UPDATE sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?');
            updateStmt.run(sessionId);
        } catch (error) {
            logger.error('Failed to add message:', error);
        }
    }

    /**
     * Get recent messages
     */
    getHistory(sessionId, limit = 8) {
        try {
            const stmt = this.db.prepare(`
        SELECT role, content 
        FROM messages 
        WHERE session_id = ? 
        ORDER BY created_at DESC 
        LIMIT ?
      `);

            const rows = stmt.all(sessionId, limit);
            // Return in chronological order (oldest -> newest) for Model
            return rows.reverse().map(row => ({
                role: row.role,
                content: row.content
            }));
        } catch (error) {
            logger.error('Failed to get history:', error);
            return [];
        }
    }

    /**
     * Get all sessions for the sidebar list
     * Ordered by most recently updated
     */
    getAllSessions() {
        try {
            const stmt = this.db.prepare(`
        SELECT id, title, created_at, updated_at 
        FROM sessions 
        ORDER BY updated_at DESC
      `);
            return stmt.all();
        } catch (error) {
            logger.error('Failed to get all sessions:', error);
            return [];
        }
    }

    /**
     * Get full history
     */
    getFullHistory(sessionId) {
        try {
            const stmt = this.db.prepare(`
        SELECT role, content, created_at, sources 
        FROM messages 
        WHERE session_id = ? 
        ORDER BY created_at ASC
      `);
            return stmt.all(sessionId).map(row => ({
                ...row,
                sources: row.sources ? JSON.parse(row.sources) : null
            }));
        } catch (error) {
            logger.error('Failed to get full history:', error);
            return [];
        }
    }

    deleteSession(sessionId) {
        try {
            const stmt = this.db.prepare('DELETE FROM sessions WHERE id = ?');
            const result = stmt.run(sessionId);
            return result.changes > 0;
        } catch (error) {
            logger.error('Failed to delete session:', error);
            throw error;
        }
    }

    updateSessionTitle(sessionId, title) {
        try {
            const stmt = this.db.prepare('UPDATE sessions SET title = ? WHERE id = ?');
            const result = stmt.run(title, sessionId);
            return result.changes > 0;
        } catch (error) {
            logger.error('Failed to update session title:', error);
            throw error;
        }
    }

    clearSession(sessionId) {
        return this.deleteSession(sessionId);
    }
}

export default new HistoryService();
