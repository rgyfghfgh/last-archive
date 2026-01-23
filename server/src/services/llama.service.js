import fetch from 'node-fetch';
import config from '../config/environment.js';
import logger from '../utils/logger.js';

class LlamaService {
  constructor() {
    this.baseUrl = config.llama.baseUrl;
    this.model = config.llama.model;
  }

  /**
   * Stream a chat completion
   * @param {Array} messages
   * @param {Function} onToken
   * @param {Object} options
   */
  async streamChat(messages, onToken, options = {}) {
    try {
      const response = await fetch(`${this.baseUrl}/v1/chat/completions`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          model: this.model,
          messages,
          stream: true,
          temperature: options.temperature || 0.7,
          max_tokens: options.maxTokens || 500,
          num_ctx: options.numCtx || 2048,
        }),
      });

      if (!response.ok) {
        throw new Error(`LLaMA API error: ${response.status} ${response.statusText}`);
      }

      // Process chunks in real-time
      let buffer = '';

      for await (const chunk of response.body) {
        buffer += chunk.toString();

        // Process complete lines (SSE messages end with \n\n)
        const parts = buffer.split('\n\n');
        buffer = parts.pop() || ''; // Keep incomplete part in buffer

        for (const part of parts) {
          const lines = part.split('\n');

          for (const line of lines) {
            const trimmed = line.trim();

            if (trimmed.startsWith('data: ')) {
              const dataStr = trimmed.slice(6);

              if (dataStr === '[DONE]') {
                logger.debug('Stream done');
                return;
              }

              try {
                const json = JSON.parse(dataStr);

                if (json.choices && json.choices.length > 0) {
                  const content = json.choices[0].delta?.content;
                  if (content) {
                    onToken(content);
                  }
                }
              } catch (e) {
                logger.debug(`Parse error: ${dataStr.substring(0, 50)}`);
              }
            }
          }
        }
      }

      // Process any remaining buffer
      if (buffer.trim()) {
        const lines = buffer.split('\n');
        for (const line of lines) {
          const trimmed = line.trim();
          if (trimmed.startsWith('data: ') && trimmed !== 'data: [DONE]') {
            try {
              const json = JSON.parse(trimmed.slice(6));
              if (json.choices?.[0]?.delta?.content) {
                onToken(json.choices[0].delta.content);
              }
            } catch (e) { }
          }
        }
      }

    } catch (error) {
      logger.error('Stream error:', error.message);
      throw error;
    }
  }

  /**
   * Non-streaming chat
   */
  async chat(messages, options = {}) {
    try {
      const response = await fetch(`${this.baseUrl}/v1/chat/completions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: this.model,
          messages,
          stream: false,
          temperature: options.temperature || 0.7,
          max_tokens: options.maxTokens || 2000,
        }),
      });

      if (!response.ok) throw new Error(`LLaMA API error: ${response.status}`);

      const data = await response.json();
      return data.choices[0].message.content;
    } catch (error) {
      logger.error('Chat error:', error.message);
      throw error;
    }
  }

  async enhanceQuery(userQuery) {
    const systemPrompt = `Refine the user question for better search. Keep it short.`;
    const messages = [
      { role: 'system', content: systemPrompt },
      { role: 'user', content: userQuery },
    ];
    const enhanced = await this.chat(messages, { temperature: 0.3 });
    return enhanced.trim();
  }

  async generateStreamResponse(userQuery, context, history = [], onToken) {
    const systemPrompt = `You are an assistant. Answer using the context. Be short and fact-based.`;

    const contextText = this.formatContext(context);

    if (contextText.length < 100) {
      onToken("I don't have sufficient relevant information to answer this.");
      return;
    }

    // Include context in the user's latest message for 
    const userMessage = `Context:\n${contextText}\n\nQuestion: ${userQuery}\n\nProvide a helpful answer based on the context above:`;

    // Construct format: [System, ...History, UserWithContext]
    const messages = [
      { role: 'system', content: systemPrompt },
      ...history,
      { role: 'user', content: userMessage },
    ];

    logger.info(`Request: ${history.length} messages, context: ${contextText.length}`);
    await this.streamChat(messages, onToken, {
      temperature: 0.1,
      maxTokens: 500,
      numCtx: 2048
    });
  }

  formatContext(context) {
    let formatted = '';

    if (context.pages && context.pages.length > 0) {
      formatted += '=== WEB PAGES ===\n\n';
      context.pages.forEach((page, idx) => {
        formatted += `[Page ${idx + 1}] Title: ${page.title}\nContent: ${page.content}\n\n`;
      });
    }

    if (context.pdfs && context.pdfs.length > 0) {
      formatted += '=== PDF DOCUMENTS ===\n\n';
      context.pdfs.forEach((pdf, idx) => {
        formatted += `[PDF ${idx + 1}] Source: ${pdf.url}\nContent: ${pdf.content}\n\n`;
      });
    }

    return formatted;
  }
}

export default new LlamaService();