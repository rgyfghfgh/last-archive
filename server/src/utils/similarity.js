/**
 * Calculate cosine similarity between two vectors
 * @param {number[]} vecA - First vector
 * @param {number[]} vecB - Second vector
 * @returns {number} Similarity score between -1 and 1
 */
export function cosineSimilarity(vecA, vecB) {
  if (vecA.length !== vecB.length) {
    throw new Error('Vectors must have the same length');
  }

  let dotProduct = 0;
  let normA = 0;
  let normB = 0;

  for (let i = 0; i < vecA.length; i++) {
    dotProduct += vecA[i] * vecB[i];
    normA += vecA[i] * vecA[i];
    normB += vecB[i] * vecB[i];
  }

  const magnitude = Math.sqrt(normA) * Math.sqrt(normB);
  
  if (magnitude === 0) {
    return 0;
  }

  return dotProduct / magnitude;
}

/**
 * Calculate similarities between a query vector and multiple document vectors
 * @param {number[]} queryVector - Query embedding
 * @param {Array<{embedding: number[], ...}>} documents - Documents with embeddings
 * @returns {Array<{similarity: number, ...}>} Documents with similarity scores
 */
export function calculateSimilarities(queryVector, documents) {
  return documents.map(doc => ({
    ...doc,
    similarity: cosineSimilarity(queryVector, doc.embedding),
  }));
}

/**
 * Rerank documents by similarity and return top K
 * @param {number[]} queryVector - Query embedding
 * @param {Array<{embedding: number[], ...}>} documents - Documents with embeddings
 * @param {number} topK - Number of top documents to return
 * @returns {Array} Top K documents sorted by similarity
 */
export function rerankBySimilarity(queryVector, documents, topK = 5) {
  const withSimilarity = calculateSimilarities(queryVector, documents);
  
  return withSimilarity
    .sort((a, b) => b.similarity - a.similarity)
    .slice(0, topK);
}

export default {
  cosineSimilarity,
  calculateSimilarities,
  rerankBySimilarity,
};