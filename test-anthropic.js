const Anthropic = require('@anthropic-ai/sdk');

// 设置环境变量
process.env.ANTHROPIC_BASE_URL = 'http://43.153.213.46:8486';
process.env.ANTHROPIC_AUTH_TOKEN = 'sk-lT1AkzXQCmyPBm9T9AiHWxIxaWGdKyz3ylN0QAgp5mFF3rMh';

// 创建 Anthropic 客户端
const client = new Anthropic({
  baseURL: process.env.ANTHROPIC_BASE_URL,
  authToken: process.env.ANTHROPIC_AUTH_TOKEN
});

async function test() {
  try {
    console.log('=== 测试模型列表 ===');
    const models = await client.models.list();
    console.log('可用模型:');
    models.data.forEach(model => {
      console.log(`- ${model.id}`);
    });

    console.log('\n=== 测试消息 ===');
    const message = await client.messages.create({
      model: 'claude-opus-4-6',
      messages: [{ role: 'user', content: '你好，测试消息' }],
      max_tokens: 100
    });
    console.log('消息响应:', message);
  } catch (error) {
    console.error('错误:', error);
  }
}

test();
