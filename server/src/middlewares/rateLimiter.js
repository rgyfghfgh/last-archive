import rateLimit from 'express-rate-limit';
import config from '../config/environment.js';

export const searchLimiter = rateLimit({
  windowMs: config.rateLimit.windowMs,
  max: config.rateLimit.maxRequests,
  message: {
    success: false,
    error: 'Too many requests, please try again later.',
  },
  standardHeaders: true,
  legacyHeaders: false,
});

export const generalLimiter = rateLimit({
  windowMs: 60000, // 1 minute
  max: 100, // 100 requests per minute
  message: {
    success: false,
    error: 'Rate limit exceeded',
  },
});

export default {
  searchLimiter,
  generalLimiter,
};