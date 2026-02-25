import express from 'express';
import cors from 'cors';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import fs from 'fs';
import { Low } from 'lowdb';
import { JSONFile } from 'lowdb/node';
import { nanoid } from 'nanoid';

// 1. 初始化应用
const app = express();
const PORT = process.env.PORT || 3000;
const __dirname = dirname(fileURLToPath(import.meta.url));

// 2. 数据库设置 (LowDB)
const dbDir = join(__dirname, 'data');
if (!fs.existsSync(dbDir)) fs.mkdirSync(dbDir); // 确保 data 文件夹存在
const file = join(dbDir, 'db.json');
const adapter = new JSONFile(file);
const db = new Low(adapter, { keys: [], settings: {}, balanceHistory: [] }); // 默认数据结构

// 初始化数据库
await db.read();
db.data ||= { keys: [], settings: {}, auth: { username: 'admin', password: 'admin123' }, balanceHistory: [] };
if (!db.data.settings) db.data.settings = {};
if (!db.data.auth) db.data.auth = { username: 'admin', password: 'admin123' };
if (!db.data.balanceHistory) db.data.balanceHistory = [];
await db.write();

// 简单的session存储（内存中）
const sessions = new Map();

// 定时发送任务
let notifyTimer = null;
let notifyIntervalTimer = null;

// 3. 中间件
app.use(cors());
app.use(express.json());

// 简单的session中间件
app.use((req, res, next) => {
    const sessionId = req.headers['x-session-id'] || req.query.sessionId;
    if (sessionId && sessions.has(sessionId)) {
        req.session = sessions.get(sessionId);
        req.sessionId = sessionId;
    }
    next();
});

// 登录验证中间件
function requireAuth(req, res, next) {
    const sessionId = req.headers['x-session-id'] || req.query.sessionId;
    if (!sessionId || !sessions.has(sessionId)) {
        return res.status(401).json({ error: '未登录', loggedIn: false });
    }
    next();
}

app.use(express.static(join(__dirname, 'public'))); // 托管静态网页

// ================= 认证相关 API =================

// 登录
app.post('/api/auth/login', async (req, res) => {
    const { username, password } = req.body;
    
    await db.read();
    const auth = db.data.auth || {};
    
    if (username === auth.username && password === auth.password) {
        // 生成session ID
        const sessionId = nanoid();
        sessions.set(sessionId, { username, loginTime: Date.now() });
        
        // 设置session过期时间（24小时）
        setTimeout(() => {
            sessions.delete(sessionId);
        }, 24 * 60 * 60 * 1000);
        
        res.json({ success: true, sessionId });
    } else {
        res.status(401).json({ error: '账号或密码错误' });
    }
});

// 检查登录状态
app.get('/api/auth/check', (req, res) => {
    const sessionId = req.headers['x-session-id'] || req.query.sessionId;
    const loggedIn = sessionId && sessions.has(sessionId);
    res.json({ loggedIn });
});

// 登出
app.post('/api/auth/logout', (req, res) => {
    const sessionId = req.headers['x-session-id'] || req.query.sessionId;
    if (sessionId) {
        sessions.delete(sessionId);
    }
    res.json({ success: true });
});

// 更新账号密码
app.post('/api/auth/update', requireAuth, async (req, res) => {
    const { username, password } = req.body;
    
    if (!username || !password) {
        return res.status(400).json({ error: '账号和密码不能为空' });
    }
    
    await db.read();
    db.data.auth = { username, password };
    await db.write();
    
    res.json({ success: true });
});

// ================= API 接口 =================

// 获取所有 Key
app.get('/api/keys', requireAuth, async (req, res) => {
    await db.read();
    res.json(db.data.keys);
});

// ================= 余额历史记录 API =================

// 保存余额历史记录
app.post('/api/balance-history', requireAuth, async (req, res) => {
    const { keyId, balance, usage, remaining } = req.body;
    
    if (keyId === undefined || balance === undefined) {
        return res.status(400).json({ error: '缺少参数' });
    }
    
    await db.read();
    
    const historyEntry = {
        id: nanoid(),
        keyId: keyId || 'all', // 'all' 表示总计
        balance: parseFloat(balance),
        usage: parseFloat(usage) || 0,
        remaining: parseFloat(remaining) || 0,
        timestamp: new Date().toISOString(),
        date: new Date().toISOString().split('T')[0] // YYYY-MM-DD
    };
    
    db.data.balanceHistory.push(historyEntry);
    
    // 只保留最近90天的数据，避免数据库过大
    const cutoffDate = new Date();
    cutoffDate.setDate(cutoffDate.getDate() - 90);
    db.data.balanceHistory = db.data.balanceHistory.filter(entry => {
        return new Date(entry.timestamp) > cutoffDate;
    });
    
    await db.write();
    res.json({ success: true, entry: historyEntry });
});

// 获取余额历史记录
app.get('/api/balance-history', requireAuth, async (req, res) => {
    const { keyId, startDate, endDate } = req.query;
    
    await db.read();
    let history = db.data.balanceHistory || [];
    
    // 按密钥过滤
    if (keyId && keyId !== 'all') {
        history = history.filter(entry => entry.keyId === keyId);
    } else if (keyId === 'all') {
        // 只返回总计数据
        history = history.filter(entry => entry.keyId === 'all');
    }
    
    // 按日期过滤
    if (startDate) {
        history = history.filter(entry => entry.date >= startDate);
    }
    if (endDate) {
        history = history.filter(entry => entry.date <= endDate);
    }
    
    // 按时间排序（从旧到新）
    history.sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));
    
    res.json(history);
});

// 添加 Key
app.post('/api/keys', requireAuth, async (req, res) => {
    const { name, key, quota, group, exhaustedThreshold, warningThreshold, purchaseRate, sellRate } = req.body;
    if (!name || !key) return res.status(400).json({ error: '缺少参数' });

    const newKey = {
        id: nanoid(), // 生成唯一ID
        name,
        key,
        quota: parseFloat(quota) || 0,
        group: group || '默认分组',
        createTime: new Date()
    };
    
    // 添加个性化阈值设置（如果有）
    if (exhaustedThreshold !== undefined) newKey.exhaustedThreshold = exhaustedThreshold;
    if (warningThreshold !== undefined) newKey.warningThreshold = warningThreshold;
    
    // 添加个性化价格设置（如果有）
    if (purchaseRate !== undefined) newKey.purchaseRate = purchaseRate;
    if (sellRate !== undefined) newKey.sellRate = sellRate;

    db.data.keys.push(newKey);
    await db.write();
    res.json(newKey);
});

// 更新 Key
app.put('/api/keys/:id', requireAuth, async (req, res) => {
    const { id } = req.params;
    const { name, key, quota, group, exhaustedThreshold, warningThreshold, purchaseRate, sellRate } = req.body;
    
    if (!name || !key) return res.status(400).json({ error: '缺少参数' });

    await db.read();
    const keyIndex = db.data.keys.findIndex(k => k.id === id);
    
    if (keyIndex === -1) {
        return res.status(404).json({ error: '密钥不存在' });
    }

    const updatedKey = {
        ...db.data.keys[keyIndex],
        name,
        key,
        quota: parseFloat(quota) || 0,
        group: group || '默认分组'
    };
    
    // 更新个性化阈值设置
    if (exhaustedThreshold !== undefined) {
        if (exhaustedThreshold === null) {
            delete updatedKey.exhaustedThreshold;
        } else {
            updatedKey.exhaustedThreshold = exhaustedThreshold;
        }
    }
    if (warningThreshold !== undefined) {
        if (warningThreshold === null) {
            delete updatedKey.warningThreshold;
        } else {
            updatedKey.warningThreshold = warningThreshold;
        }
    }
    
    // 更新个性化价格设置
    if (purchaseRate !== undefined) {
        if (purchaseRate === null) {
            delete updatedKey.purchaseRate;
        } else {
            updatedKey.purchaseRate = purchaseRate;
        }
    }
    if (sellRate !== undefined) {
        if (sellRate === null) {
            delete updatedKey.sellRate;
        } else {
            updatedKey.sellRate = sellRate;
        }
    }
    
    db.data.keys[keyIndex] = updatedKey;
    await db.write();
    res.json(db.data.keys[keyIndex]);
});

// 删除 Key
app.delete('/api/keys/:id', requireAuth, async (req, res) => {
    const { id } = req.params;
    db.data.keys = db.data.keys.filter(k => k.id !== id);
    await db.write();
    res.json({ success: true });
});

// 归档 Key
app.post('/api/keys/:id/archive', requireAuth, async (req, res) => {
    const { id } = req.params;
    await db.read();
    const keyIndex = db.data.keys.findIndex(k => k.id === id);
    
    if (keyIndex === -1) {
        return res.status(404).json({ error: '密钥不存在' });
    }
    
    db.data.keys[keyIndex].archived = true;
    db.data.keys[keyIndex].archivedTime = new Date();
    await db.write();
    res.json({ success: true });
});

// 取消归档 Key
app.delete('/api/keys/:id/archive', requireAuth, async (req, res) => {
    const { id } = req.params;
    await db.read();
    const keyIndex = db.data.keys.findIndex(k => k.id === id);
    
    if (keyIndex === -1) {
        return res.status(404).json({ error: '密钥不存在' });
    }
    
    delete db.data.keys[keyIndex].archived;
    delete db.data.keys[keyIndex].archivedTime;
    await db.write();
    res.json({ success: true });
});

// ================= 设置相关 API =================

// 获取设置
app.get('/api/settings', requireAuth, async (req, res) => {
    await db.read();
    res.json(db.data.settings || {});
});

// 保存设置
app.post('/api/settings', requireAuth, async (req, res) => {
    const { webhookUrl, enableNotify, notifyInterval, purchaseRate, sellRate, exhaustedThreshold, warningThreshold, notifyChannels } = req.body;
    
    await db.read();
    db.data.settings = {
        ...db.data.settings,
        webhookUrl: webhookUrl !== undefined ? webhookUrl : (db.data.settings?.webhookUrl || ''),
        enableNotify: enableNotify !== undefined ? enableNotify : (db.data.settings?.enableNotify || false),
        notifyInterval: notifyInterval !== undefined ? notifyInterval : (db.data.settings?.notifyInterval || 60),
        purchaseRate: purchaseRate !== undefined ? purchaseRate : (db.data.settings?.purchaseRate || 3.5),
        sellRate: sellRate !== undefined ? sellRate : (db.data.settings?.sellRate || 4.0),
        exhaustedThreshold: exhaustedThreshold !== undefined ? exhaustedThreshold : (db.data.settings?.exhaustedThreshold || 2),
        warningThreshold: warningThreshold !== undefined ? warningThreshold : (db.data.settings?.warningThreshold || 20),
        notifyChannels: notifyChannels !== undefined ? notifyChannels : (db.data.settings?.notifyChannels || {})
    };
    // 删除不再使用的notifyTime字段
    if (db.data.settings.notifyTime) {
        delete db.data.settings.notifyTime;
    }
    await db.write();

    // 重启定时任务（不立即发送，等待下一个定时周期）
    startNotifyTask(false);

    res.json({ success: true, settings: db.data.settings });
});

// 保存通知模板
app.post('/api/settings/template', requireAuth, async (req, res) => {
    const { notifyTemplate, keyTemplate } = req.body;
    
    await db.read();
    db.data.settings = {
        ...db.data.settings,
        notifyTemplate: notifyTemplate !== undefined ? notifyTemplate : (db.data.settings?.notifyTemplate || ''),
        keyTemplate: keyTemplate !== undefined ? keyTemplate : (db.data.settings?.keyTemplate || '')
    };
    await db.write();

    res.json({ success: true });
});

// 保存价格配置
app.post('/api/settings/price', requireAuth, async (req, res) => {
    const { purchaseRate, sellRate } = req.body;
    
    await db.read();
    db.data.settings = {
        ...db.data.settings,
        purchaseRate: purchaseRate || 3.5,
        sellRate: sellRate || 4.0
    };
    await db.write();

    res.json({ success: true });
});

// 测试通知渠道
app.post('/api/test-channel', requireAuth, async (req, res) => {
    const { channel } = req.query;
    const config = req.body;
    
    if (!channel) {
        return res.status(400).json({ error: '请提供渠道类型' });
    }

    try {
        let result;
        const testMessage = `**测试消息**\n\n这是一条测试消息，用于验证${getChannelName(channel)}配置是否正确。\n\n发送时间: ${new Date().toLocaleString('zh-CN')}`;
        
        if (channel === 'wechat') {
            result = await sendWeChatMessage(config.webhook, {
                msgtype: 'markdown',
                markdown: { content: testMessage }
            });
        } else if (channel === 'dingtalk') {
            result = await sendDingTalkMessage(config.webhook, testMessage);
        } else if (channel === 'feishu') {
            result = await sendFeishuMessage(config.webhook, testMessage);
        } else if (channel === 'email') {
            result = await sendEmailMessage(config, '测试消息', testMessage);
        } else {
            return res.status(400).json({ error: '不支持的渠道类型' });
        }
        
        if (result.success) {
            res.json({ success: true });
        } else {
            res.status(400).json({ error: result.error || '发送失败' });
        }
    } catch (error) {
        console.error(`测试${channel}错误:`, error);
        // 确保返回JSON格式的错误
        if (!res.headersSent) {
            res.status(500).json({ error: error.message || '网络请求失败' });
        }
    }
});

function getChannelName(channel) {
    const names = {
        'wechat': '企业微信',
        'dingtalk': '钉钉',
        'feishu': '飞书',
        'email': '邮件'
    };
    return names[channel] || channel;
}

// ================= 多渠道发送消息 =================

// 发送钉钉消息
async function sendDingTalkMessage(webhookUrl, content) {
    try {
        const https = (await import('https')).default;
        const http = (await import('http')).default;
        const url = (await import('url')).default;
        const { URL } = url;
        
        const targetUrl = new URL(webhookUrl);
        const client = targetUrl.protocol === 'https:' ? https : http;
        
        const message = {
            msgtype: 'markdown',
            markdown: {
                title: 'OpenRouter余额通知',
                text: content
            }
        };
        
        const postData = JSON.stringify(message);
        
        const options = {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(postData)
            },
            rejectUnauthorized: false
        };
        
        const response = await new Promise((resolve, reject) => {
            const req = client.request(targetUrl, options, (res) => {
                let data = '';
                res.on('data', (chunk) => { data += chunk; });
                res.on('end', () => {
                    try {
                        resolve({ statusCode: res.statusCode, data: JSON.parse(data) });
                    } catch (e) {
                        resolve({ statusCode: res.statusCode, data: { errmsg: data } });
                    }
                });
            });
            
            req.on('error', (error) => {
                reject(error);
            });
            
            req.write(postData);
            req.end();
        });
        
        return { 
            success: response.statusCode === 200 && response.data.errcode === 0, 
            error: response.data.errmsg || (response.statusCode !== 200 ? `HTTP ${response.statusCode}` : null)
        };
    } catch (error) {
        console.error('发送钉钉消息错误:', error);
        return { success: false, error: error.message || '网络请求失败' };
    }
}

// 发送飞书消息
async function sendFeishuMessage(webhookUrl, content) {
    try {
        const https = (await import('https')).default;
        const http = (await import('http')).default;
        const url = (await import('url')).default;
        const { URL } = url;
        
        const targetUrl = new URL(webhookUrl);
        const client = targetUrl.protocol === 'https:' ? https : http;
        
        const message = {
            msg_type: 'interactive',
            card: {
                config: {
                    wide_screen_mode: true
                },
                header: {
                    title: {
                        tag: 'plain_text',
                        content: 'OpenRouter余额通知'
                    },
                    template: 'blue'
                },
                elements: [
                    {
                        tag: 'div',
                        text: {
                            tag: 'lark_md',
                            content: content
                        }
                    }
                ]
            }
        };
        
        const postData = JSON.stringify(message);
        
        const options = {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(postData)
            },
            rejectUnauthorized: false
        };
        
        const response = await new Promise((resolve, reject) => {
            const req = client.request(targetUrl, options, (res) => {
                let data = '';
                res.on('data', (chunk) => { data += chunk; });
                res.on('end', () => {
                    try {
                        resolve({ statusCode: res.statusCode, data: JSON.parse(data) });
                    } catch (e) {
                        resolve({ statusCode: res.statusCode, data: { msg: data } });
                    }
                });
            });
            
            req.on('error', (error) => {
                reject(error);
            });
            
            req.write(postData);
            req.end();
        });
        
        return { 
            success: response.statusCode === 200 && response.data.code === 0, 
            error: response.data.msg || (response.statusCode !== 200 ? `HTTP ${response.statusCode}` : null)
        };
    } catch (error) {
        console.error('发送飞书消息错误:', error);
        return { success: false, error: error.message || '网络请求失败' };
    }
}

// 发送邮件消息
async function sendEmailMessage(config, subject, content) {
    try {
        // 使用Node.js内置模块发送邮件（简化版，使用SMTP）
        const https = (await import('https')).default;
        const http = (await import('http')).default;
        const tls = (await import('tls')).default;
        const net = (await import('net')).default;
        
        // 简单的SMTP发送（使用第三方服务或直接SMTP）
        // 这里使用一个简化的SMTP客户端实现
        return await sendSMTPEmail(config, subject, content);
    } catch (error) {
        console.error('发送邮件错误:', error);
        return { success: false, error: error.message || '邮件发送失败' };
    }
}

// 简单的SMTP邮件发送
async function sendSMTPEmail(config, subject, htmlContent) {
    try {
        // 注意：这是一个简化的SMTP实现
        // 生产环境建议使用nodemailer库: npm install nodemailer
        // 或者使用第三方邮件服务API（如SendGrid、Mailgun等）
        
        const net = (await import('net')).default;
        const tls = (await import('tls')).default;
        
        const port = parseInt(config.port) || 587;
        const isSecure = port === 465;
        const useTLS = port === 587;
        
        return new Promise((resolve) => {
            let socket;
            let data = '';
            let step = -1;
            let resolved = false;
            
            const resolveOnce = (result) => {
                if (!resolved) {
                    resolved = true;
                    resolve(result);
                }
            };
            
            const connect = () => {
                if (isSecure) {
                    socket = tls.connect(port, config.host, { rejectUnauthorized: false }, () => {
                        step = 0;
                        socket.write(`EHLO ${config.host}\r\n`);
                    });
                } else {
                    socket = net.createConnection(port, config.host, () => {
                        step = 0;
                        socket.write(`EHLO ${config.host}\r\n`);
                    });
                }
                
                socket.on('data', (chunk) => {
                    data += chunk.toString();
                    const lines = data.split('\r\n');
                    data = lines.pop() || '';
                    
                    for (const line of lines) {
                        if (line.trim()) {
                            handleSMTPResponse(line);
                        }
                    }
                });
                
                socket.on('error', (err) => {
                    resolveOnce({ success: false, error: err.message });
                });
                
                socket.on('close', () => {
                    if (!resolved) {
                        resolveOnce({ success: false, error: '连接已关闭' });
                    }
                });
            };
            
            const handleSMTPResponse = (response) => {
                const code = parseInt(response.substring(0, 3));
                
                if (code === 220 && step === 0) {
                    // 服务器就绪
                    if (useTLS && !isSecure && !socket.encrypted) {
                        socket.write('STARTTLS\r\n');
                        step = -1;
                    } else {
                        socket.write(`AUTH LOGIN\r\n`);
                        step = 1;
                    }
                } else if (response.startsWith('250') && step === 0) {
                    if (useTLS && !socket.encrypted) {
                        // STARTTLS响应
                        socket.write(`AUTH LOGIN\r\n`);
                        step = 1;
                    } else {
                        socket.write(`AUTH LOGIN\r\n`);
                        step = 1;
                    }
                } else if (code === 334 && step === 1) {
                    socket.write(Buffer.from(config.from).toString('base64') + '\r\n');
                    step = 2;
                } else if (code === 334 && step === 2) {
                    socket.write(Buffer.from(config.password).toString('base64') + '\r\n');
                    step = 3;
                } else if (code === 235 && step === 3) {
                    socket.write(`MAIL FROM:<${config.from}>\r\n`);
                    step = 4;
                } else if (code === 250 && step === 4) {
                    socket.write(`RCPT TO:<${config.to}>\r\n`);
                    step = 5;
                } else if (code === 250 && step === 5) {
                    socket.write(`DATA\r\n`);
                    step = 6;
                } else if (code === 354 && step === 6) {
                    const emailContent = `From: ${config.from}\r\nTo: ${config.to}\r\nSubject: ${subject}\r\nContent-Type: text/html; charset=utf-8\r\n\r\n${htmlContent.replace(/\n/g, '<br>')}\r\n.\r\n`;
                    socket.write(emailContent);
                    step = 7;
                } else if (code === 250 && step === 7) {
                    socket.write(`QUIT\r\n`);
                    socket.end();
                    resolveOnce({ success: true });
                } else if (code >= 400) {
                    socket.destroy();
                    resolveOnce({ success: false, error: `SMTP错误: ${response}` });
                }
            };
            
            connect();
            
            // 超时处理
            setTimeout(() => {
                if (socket && !socket.destroyed && !resolved) {
                    socket.destroy();
                    resolveOnce({ success: false, error: '连接超时' });
                }
            }, 15000);
        });
    } catch (error) {
        return { success: false, error: error.message };
    }
}

// ================= 企业微信发送消息 =================
async function sendWeChatMessage(webhookUrl, message) {
    try {
        // 验证webhook URL格式
        if (!webhookUrl || typeof webhookUrl !== 'string') {
            return { success: false, error: 'Webhook URL不能为空' };
        }
        
        const https = (await import('https')).default;
        const http = (await import('http')).default;
        const url = (await import('url')).default;
        const { URL } = url;
        
        let targetUrl;
        try {
            targetUrl = new URL(webhookUrl);
        } catch (e) {
            return { success: false, error: `Webhook URL格式不正确: ${e.message}` };
        }
        
        const client = targetUrl.protocol === 'https:' ? https : http;
        
        const postData = JSON.stringify(message);
        
        const options = {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(postData)
            },
            hostname: targetUrl.hostname,
            port: targetUrl.port || (targetUrl.protocol === 'https:' ? 443 : 80),
            path: targetUrl.pathname + targetUrl.search
        };
        
        // 如果是https，添加证书选项
        if (targetUrl.protocol === 'https:') {
            options.rejectUnauthorized = false; // 允许自签名证书
        }
        
        const response = await new Promise((resolve, reject) => {
            const req = client.request(options, (res) => {
                let data = '';
                res.on('data', (chunk) => { data += chunk; });
                res.on('end', () => {
                    try {
                        resolve({ statusCode: res.statusCode, data: JSON.parse(data) });
                    } catch (e) {
                        // 如果无法解析为JSON，返回原始数据
                        resolve({ statusCode: res.statusCode, data: { errmsg: data.substring(0, 200) } });
                    }
                });
            });
            
            req.on('error', (error) => {
                reject(error);
            });
            
            req.write(postData);
            req.end();
        });
        
        return { 
            success: response.statusCode === 200 && response.data.errcode === 0, 
            error: response.data.errmsg || (response.statusCode !== 200 ? `HTTP ${response.statusCode}` : null)
        };
    } catch (error) {
        console.error('发送企业微信消息错误:', error);
        return { success: false, error: error.message || '网络请求失败' };
    }
}

// 获取当前数据并生成消息
async function generateNotifyMessage() {
    await db.read();
    const allKeys = db.data.keys || [];
    
    // 过滤掉归档密钥和失效分组（不再请求官方信息）
    const keys = allKeys.filter(key => !key.archived && (key.group || '默认分组') !== '失效分组');
    
    if (keys.length === 0) {
        return null;
    }

    // 获取所有key的数据
    let totalUsage = 0;
    let totalRemaining = 0;
    let totalDaily = 0;
    const keyDetails = [];

    for (const key of keys) {
        try {
            // 使用https模块发送请求
            const https = (await import('https')).default;
            const json = await new Promise((resolve, reject) => {
                const req = https.request('https://openrouter.ai/api/v1/key', {
                    method: 'GET',
                    headers: {
                        'Authorization': `Bearer ${key.key}`
                    },
                    rejectUnauthorized: false // 允许自签名证书
                }, (res) => {
                    let data = '';
                    res.on('data', (chunk) => { data += chunk; });
                    res.on('end', () => {
                        try {
                            resolve(JSON.parse(data));
                        } catch (e) {
                            reject(new Error('解析响应失败: ' + data));
                        }
                    });
                });
                
                req.on('error', (error) => {
                    reject(error);
                });

                req.setTimeout(10000, () => {
                    req.destroy(new Error('请求超时(10s)'));
                });
                
                req.end();
            });
            
            if (!json.error && json.data) {
                const usage = json.data.usage || 0;
                const daily = json.data.usage_daily || 0;
                const remaining = Math.max(0, key.quota - usage);
                
                // 从设置中获取价格（优先使用个性化价格，否则使用系统设置）
                const settings = db.data.settings || {};
                const purchaseRate = key.purchaseRate !== undefined ? key.purchaseRate : (settings.purchaseRate || 3.5);
                const sellRate = key.sellRate !== undefined ? key.sellRate : (settings.sellRate || 4.0);
                
                // 获取阈值设置（优先使用个性化阈值，否则使用全局阈值）
                const exhaustedThreshold = key.exhaustedThreshold !== undefined ? key.exhaustedThreshold : (settings.exhaustedThreshold || 2);
                const warningThreshold = key.warningThreshold !== undefined ? key.warningThreshold : (settings.warningThreshold || 20);
                
                // 检查是否耗尽，如果耗尽且未归档，自动归档（无限额度跳过）
                if (key.quota && remaining < exhaustedThreshold && !key.archived) {
                    // 自动归档耗尽的密钥
                    await db.read();
                    const keyIndex = db.data.keys.findIndex(k => k.id === key.id);
                    if (keyIndex >= 0) {
                        db.data.keys[keyIndex].archived = true;
                        db.data.keys[keyIndex].archivedTime = new Date();
                        await db.write();
                        console.log(`🔒 密钥 "${key.name}" 已自动归档（余额耗尽: $${remaining.toFixed(2)} < $${exhaustedThreshold}）`);
                        // 更新当前key对象的状态
                        key.archived = true;
                        key.archivedTime = db.data.keys[keyIndex].archivedTime;
                        // 跳过这个密钥，不加入通知消息
                        continue;
                    }
                }
                
                totalUsage += usage;
                totalDaily += daily;
                totalRemaining += remaining;
                
                const usagePercent = key.quota > 0 ? (usage / key.quota * 100).toFixed(1) : 0;
                const remainingPercent = key.quota > 0 ? (remaining / key.quota * 100).toFixed(0) : 0;
                const totalProfit = usage * (sellRate - purchaseRate);
                const dailyProfit = daily * (sellRate - purchaseRate);
                
                // 状态判断：使用配置的阈值
                let status = '✅ 健康';
                if (remaining < exhaustedThreshold) {
                    status = '❌ 耗尽';
                } else if (remaining < warningThreshold) {
                    status = '⚠️ 警告';
                }
                
                keyDetails.push({
                    name: key.name,
                    key: key.key,
                    quota: key.quota.toFixed(2),
                    usage: usage.toFixed(2),
                    remaining: remaining.toFixed(2),
                    daily: daily.toFixed(2),
                    percent: usagePercent,
                    remainingPercent: remainingPercent,
                    totalProfit: totalProfit.toFixed(2),
                    dailyProfit: dailyProfit.toFixed(2),
                    status: status
                });
            }
        } catch (e) {
            keyDetails.push({
                name: key.name,
                status: '❌ 错误',
                error: e.message
            });
        }
    }

    // 计算总利润（基于每个密钥的个性化价格）
    const settings = db.data.settings || {};
    let totalProfit = 0;
    let totalDailyProfit = 0;
    
    // 遍历所有密钥，使用各自的个性化价格计算利润
    for (let i = 0; i < keys.length; i++) {
        const key = keys[i];
        const keyDetail = keyDetails[i];
        
        if (!keyDetail || keyDetail.error) continue;
        
        // 获取该密钥的价格（优先使用个性化价格，否则使用系统设置）
        const keyPurchaseRate = key.purchaseRate !== undefined ? key.purchaseRate : (settings.purchaseRate || 3.5);
        const keySellRate = key.sellRate !== undefined ? key.sellRate : (settings.sellRate || 4.0);
        
        // 使用该密钥的实际消耗和价格计算利润
        const keyUsage = parseFloat(keyDetail.usage) || 0;
        const keyDaily = parseFloat(keyDetail.daily) || 0;
        
        totalProfit += keyUsage * (keySellRate - keyPurchaseRate);
        totalDailyProfit += keyDaily * (keySellRate - keyPurchaseRate);
    }
    
    // 用于显示的系统价格（用于模板变量）
    const purchaseRate = settings.purchaseRate || 3.5;
    const sellRate = settings.sellRate || 4.0;
    
    // 格式化日期时间 (MM-DD HH:mm)
    const now = new Date();
    const month = String(now.getMonth() + 1).padStart(2, '0');
    const day = String(now.getDate()).padStart(2, '0');
    const hours = String(now.getHours()).padStart(2, '0');
    const minutes = String(now.getMinutes()).padStart(2, '0');
    const timeStr = `${month}-${day} ${hours}:${minutes}`;
    
    // 计算总计额度
    let totalQuota = 0;
    keyDetails.forEach(k => {
        if (!k.error) {
            totalQuota += parseFloat(k.quota);
        }
    });
    const totalRemainingPercent = totalQuota > 0 ? ((totalRemaining / totalQuota) * 100).toFixed(0) : 0;
    
    // 获取通知模板
    const notifyTemplate = settings.notifyTemplate || '';
    const keyTemplate = settings.keyTemplate || '';
    
    let content;
    
    if (notifyTemplate) {
        // 使用自定义模板
        content = notifyTemplate;
        
        // 生成密钥详情列表
        let keysContent = '';
        keyDetails.forEach((k, index) => {
            if (k.error) {
                let keyText = keyTemplate || `{{index}}. {{name}} --> ❌ 错误: {{error}}`;
                keyText = keyText.replace(/\{\{index\}\}/g, (index + 1).toString())
                                 .replace(/\{\{name\}\}/g, k.name)
                                 .replace(/\{\{error\}\}/g, k.error);
                keysContent += keyText + '\n';
            } else {
                const remainingCNY = (parseFloat(k.remaining) * purchaseRate).toFixed(2);
                let keyText = keyTemplate || `{{index}}. {{name}} --> 余额 [\${{remaining}}] x {{purchaseRate}} = [¥{{remainingCNY}}]  百分比 ≈ ({{remainingPercent}}%)\n    L 密钥--> [ {{key}} ]\n    L 额度--> [\${{quota}}]\n    L 总消耗 -\${{usage}}  (总利润：{{profit}}¥)\n    L 今日消耗 -\${{daily}}  (今日利润：{{dailyProfit}}¥)\n    L 状态：{{status}}`;
                
                keyText = keyText.replace(/\{\{index\}\}/g, (index + 1).toString())
                                 .replace(/\{\{name\}\}/g, k.name)
                                 .replace(/\{\{remaining\}\}/g, k.remaining)
                                 .replace(/\{\{remainingCNY\}\}/g, remainingCNY)
                                 .replace(/\{\{remainingPercent\}\}/g, k.remainingPercent)
                                 .replace(/\{\{key\}\}/g, k.key)
                                 .replace(/\{\{quota\}\}/g, k.quota)
                                 .replace(/\{\{usage\}\}/g, k.usage)
                                 .replace(/\{\{profit\}\}/g, k.totalProfit)
                                 .replace(/\{\{daily\}\}/g, k.daily)
                                 .replace(/\{\{dailyProfit\}\}/g, k.dailyProfit)
                                 .replace(/\{\{status\}\}/g, k.status)
                                 .replace(/\{\{purchaseRate\}\}/g, purchaseRate.toString());
                
                keysContent += keyText;
                if (index < keyDetails.length - 1) {
                    keysContent += '\n';
                }
            }
        });
        
        // 替换模板变量
        const totalRemainingCNY = (totalRemaining * purchaseRate).toFixed(2);
        content = content.replace(/\{\{date\}\}/g, timeStr)
                        .replace(/\{\{totalQuota\}\}/g, totalQuota.toFixed(2))
                        .replace(/\{\{totalUsage\}\}/g, totalUsage.toFixed(2))
                        .replace(/\{\{totalRemaining\}\}/g, totalRemaining.toFixed(2))
                        .replace(/\{\{totalRemainingCNY\}\}/g, totalRemainingCNY)
                        .replace(/\{\{totalRemainingPercent\}\}/g, totalRemainingPercent)
                        .replace(/\{\{totalDaily\}\}/g, totalDaily.toFixed(2))
                        .replace(/\{\{totalProfit\}\}/g, totalProfit.toFixed(2))
                        .replace(/\{\{totalDailyProfit\}\}/g, totalDailyProfit.toFixed(2))
                        .replace(/\{\{purchaseRate\}\}/g, purchaseRate.toString())
                        .replace(/\{\{sellRate\}\}/g, sellRate.toString())
                        .replace(/\{\{keys\}\}/g, keysContent);
    } else {
        // 使用默认模板
        content = `📊 OpenRouter · ${timeStr}    今日收购价格： ${purchaseRate}¥    出售价格： ${sellRate}¥\n`;
        content += `━━━━━━━━━━━━━━━━━━━━\n\n`;
        
        // 列出每个账号
        keyDetails.forEach((k, index) => {
            if (k.error) {
                content += `${index + 1}. ${k.name} --> ❌ 错误: ${k.error}\n`;
            } else {
                const remainingCNY = (parseFloat(k.remaining) * purchaseRate).toFixed(2);
                content += `${index + 1}. ${k.name} --> 余额 [$${k.remaining}] x ${purchaseRate} = [¥${remainingCNY}]  百分比 ≈ (${k.remainingPercent}%)\n`;
                content += `    L 密钥--> [ ${k.key} ]\n`;
                content += `    L 额度--> [$${k.quota}]\n`;
                content += `    L 总消耗 -$${k.usage}  (总利润：${k.totalProfit}¥)\n`;
                content += `    L 今日消耗 -$${k.daily}  (今日利润：${k.dailyProfit}¥)\n`;
            }
            // 每个账号后面添加空行（最后一个账号除外）
            if (index < keyDetails.length - 1) {
                content += `\n`;
            }
        });
        
        content += `━━━━━━━━━━━━━━━━━━━━\n`;
        content += `💰 合计\n`;
        content += `    L 额度 [$${totalQuota.toFixed(2)}]\n`;
        const totalRemainingCNY = (totalRemaining * purchaseRate).toFixed(2);
        content += `    L 剩余 [$${totalRemaining.toFixed(2)}] x ${purchaseRate} = [¥${totalRemainingCNY}]  百分比 ≈ (${totalRemainingPercent}%)\n`;
        content += `    L 总消耗 [$${totalUsage.toFixed(2)}]  总利润：${totalProfit.toFixed(2)}¥\n`;
        content += `    L 今日消耗 [$${totalDaily.toFixed(2)}]  今日利润：${totalDailyProfit.toFixed(2)}¥\n`;
    }

    return {
        msgtype: 'markdown',
        markdown: { content },
        plainText: content // 用于邮件等纯文本渠道
    };
}

// 启动定时发送任务
async function startNotifyTask(immediateFirst = false) {
    // 清除旧的定时器
    if (notifyTimer) clearTimeout(notifyTimer);
    if (notifyIntervalTimer) clearInterval(notifyIntervalTimer);

    await db.read();
    const settings = db.data.settings || {};
    const enableNotify = settings.enableNotify;
    const notifyInterval = settings.notifyInterval || 60;
    const notifyChannels = settings.notifyChannels || {};

    // 检查是否至少启用了一个渠道
    const hasEnabledChannel = Object.values(notifyChannels).some(ch => ch?.enabled);
    
    if (!enableNotify || !hasEnabledChannel) {
        console.log('定时发送未启用或未配置通知渠道');
        return;
    }
    
    // 发送消息的函数
    const sendMessage = async () => {
        const message = await generateNotifyMessage();
        if (!message) return;
        
        await db.read();
        const settings = db.data.settings || {};
        const notifyChannels = settings.notifyChannels || {};
        
        // 发送到所有启用的渠道
        const sendPromises = [];
        
        // 企业微信
        if (notifyChannels.wechat?.enabled && notifyChannels.wechat?.webhook) {
            sendPromises.push(
                sendWeChatMessage(notifyChannels.wechat.webhook, message).then(result => {
                    if (result.success) {
                        console.log('✅ 企业微信消息发送成功');
                    } else {
                        console.error('❌ 企业微信消息发送失败:', result.error);
                    }
                })
            );
        }
        
        // 钉钉
        if (notifyChannels.dingtalk?.enabled && notifyChannels.dingtalk?.webhook) {
            sendPromises.push(
                sendDingTalkMessage(notifyChannels.dingtalk.webhook, message.plainText || message.markdown.content).then(result => {
                    if (result.success) {
                        console.log('✅ 钉钉消息发送成功');
                    } else {
                        console.error('❌ 钉钉消息发送失败:', result.error);
                    }
                })
            );
        }
        
        // 飞书
        if (notifyChannels.feishu?.enabled && notifyChannels.feishu?.webhook) {
            sendPromises.push(
                sendFeishuMessage(notifyChannels.feishu.webhook, message.plainText || message.markdown.content).then(result => {
                    if (result.success) {
                        console.log('✅ 飞书消息发送成功');
                    } else {
                        console.error('❌ 飞书消息发送失败:', result.error);
                    }
                })
            );
        }
        
        // 邮件
        if (notifyChannels.email?.enabled && notifyChannels.email?.host) {
            sendPromises.push(
                sendEmailMessage(notifyChannels.email, 'OpenRouter余额通知', message.plainText || message.markdown.content).then(result => {
                    if (result.success) {
                        console.log('✅ 邮件发送成功');
                    } else {
                        console.error('❌ 邮件发送失败:', result.error);
                    }
                })
            );
        }
        
        // 等待所有发送完成
        await Promise.allSettled(sendPromises);
    };
    
    if (immediateFirst) {
        // 立即发送第一次（用于刚设置好时）
        console.log(`定时发送已启动，立即发送第一次，之后每 ${notifyInterval} 分钟发送一次`);
        
        // 立即发送
        await sendMessage();
        
        // 然后按照间隔发送
        notifyIntervalTimer = setInterval(sendMessage, notifyInterval * 60 * 1000);
    } else {
        // 按照设置的定时发送间隔来发送通知，不在保存后立即发送
        console.log(`定时发送已启动，将在 ${notifyInterval} 分钟后首次发送，之后每 ${notifyInterval} 分钟发送一次`);
        
        // 按照设置的间隔定时发送
        notifyIntervalTimer = setInterval(sendMessage, notifyInterval * 60 * 1000);
    }
}

// 4. 启动服务
app.listen(PORT, () => {
    console.log(`\n🚀 服务已启动! 请访问: http://localhost:${PORT}\n`);
    // 启动定时发送任务
    startNotifyTask();
});
