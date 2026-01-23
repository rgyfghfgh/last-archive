import config from '../config/environment.js';

/**
 * Split text into chunks
 */
export function chunkText(text, chunkSize = null, overlap = null) {
  const size = chunkSize || config.search.chunkSize;
  const overlapSize = overlap || config.search.chunkOverlap;

  if (!text || text.trim().length === 0) {
    return [];
  }

  const chunks = [];
  let startIndex = 0;

  while (startIndex < text.length) {
    const endIndex = Math.min(startIndex + size, text.length);
    const chunk = text.slice(startIndex, endIndex);

    if (chunk.trim().length > 0) {
      chunks.push(chunk.trim());
    }

    if (endIndex === text.length) {
      break;
    }

    startIndex += size - overlapSize;
  }

  return chunks;
}

/**
 * Split text by sentences
 */
export function splitIntoSentences(text) {
  if (!text) return [];

  return text
    .split(/[.!?]+/)
    .map(s => s.trim())
    .filter(s => s.length > 0);
}

/**
 * Create chunks by grouping sentences
 */
export function semanticChunk(text, maxChunkSize = null) {
  const size = maxChunkSize || config.search.chunkSize;
  const sentences = splitIntoSentences(text);
  const chunks = [];
  let currentChunk = '';

  for (const sentence of sentences) {
    if (currentChunk.length + sentence.length + 1 <= size) {
      currentChunk += (currentChunk ? ' ' : '') + sentence + '.';
    } else {
      if (currentChunk) {
        chunks.push(currentChunk.trim());
      }
      currentChunk = sentence + '.';
    }
  }

  if (currentChunk) {
    chunks.push(currentChunk.trim());
  }

  return chunks;
}

/**
 * Chunk multiple documents
 * @param {Array<{content: string, ...}>} documents - Documents to chunk
 * @param {string} strategy - 'fixed' or 'semantic'
 * @returns {Array<{chunk: string, sourceDoc: object, chunkIndex: number}>}
 */
export function chunkDocuments(documents, strategy = 'semantic') {
  const allChunks = [];

  documents.forEach(doc => {
    const chunks = strategy === 'semantic'
      ? semanticChunk(doc.content)
      : chunkText(doc.content);

    chunks.forEach((chunk, index) => {
      allChunks.push({
        chunk,
        chunkIndex: index,
        sourceDoc: doc,
      });
    });
  });

  return allChunks;
}

export default {
  chunkText,
  splitIntoSentences,
  semanticChunk,
  chunkDocuments,
};