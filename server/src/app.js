import express from 'express';
import searchRoutes from './routes/search.routes.js';
import healthRoutes from './routes/health.routes.js';
import historyRoutes from './routes/history.routes.js';
import { errorHandler, notFoundHandler } from './utils/errorHandler.js';
import logger from './utils/logger.js';

const app = express();

// Middleware
app.use(express.json({ limit: '10mb' }));
app.use(express.urlencoded({ extended: true, limit: '10mb' }));

// Request logging
app.use((req, res, next) => {
  logger.info(`${req.method} ${req.url}`);
  next();
});

// Routes
// Serve static files
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
app.use(express.static(path.join(__dirname, '../public')));

// Routes
app.use('/api', healthRoutes);
app.use('/api', historyRoutes);
app.use('/api', searchRoutes);

// Root endpoint serves frontend
app.get('/', (req, res) => {
  res.sendFile(path.join(__dirname, '../public/index.html'));
});

// Error handlers (must be last)
app.use(notFoundHandler);
app.use(errorHandler);

export default app;