import app from './src/app.js';
import config from './src/config/environment.js';
import logger from './src/utils/logger.js';

const PORT = config.server.port;

const server = app.listen(PORT, () => {
  logger.info('='.repeat(50));
  logger.info(`Server started`);
  logger.info(`Port: ${PORT}`);
  logger.info(`Environment: ${config.server.env}`);
  logger.info(`Llama Service: ${config.llama.baseUrl}`);
  logger.info(`Embedding Service: ${config.embedding.baseUrl}`);
  logger.info(`Qdrant: ${config.qdrant.host}:${config.qdrant.port}`);
  logger.info('='.repeat(50));
  logger.info(`\nEndpoints:`);
  logger.info(`  POST   http://localhost:${PORT}/api/search`);
  logger.info(`  GET    http://localhost:${PORT}/api/search/test`);
  logger.info(`  GET    http://localhost:${PORT}/api/health`);
  logger.info(`  GET    http://localhost:${PORT}/api/health/detailed`);
  logger.info('='.repeat(50));
});

// Graceful shutdown
const gracefulShutdown = (signal) => {
  logger.info(`\n${signal} received, shutting down gracefully...`);

  server.close(() => {
    logger.info('HTTP server closed');
    process.exit(0);
  });

  // Force shutdown after 10 seconds
  setTimeout(() => {
    logger.error('Forced shutdown after timeout');
    process.exit(1);
  }, 10000);
};

process.on('SIGTERM', () => gracefulShutdown('SIGTERM'));
process.on('SIGINT', () => gracefulShutdown('SIGINT'));

// Handle uncaught errors
process.on('uncaughtException', (error) => {
  logger.error('Uncaught Exception:', error);
  process.exit(1);
});

process.on('unhandledRejection', (reason, promise) => {
  logger.error('Unhandled Rejection at:', promise, 'reason:', reason);
  process.exit(1);
});