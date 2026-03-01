const puppeteer = require('puppeteer-core');
const fs = require('fs');
const path = require('path');

const SESSION_FILE = path.join(process.env.HOME, '.gorkbot/browser_session.json');
const USER_DATA_DIR = path.join(process.env.HOME, '.gorkbot/browser_data');
const CHROMIUM_PATH = process.env.CHROMIUM_PATH || '/data/data/com.termux/files/usr/bin/chromium-browser';

async function run() {
  const args = process.argv.slice(2);
  const action = args[0];
  const params = JSON.parse(args[1] || '{}');

  const browser = await puppeteer.launch({
    executablePath: CHROMIUM_PATH,
    headless: 'new',
    userDataDir: USER_DATA_DIR,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--single-process']
  });

  const pages = await browser.pages();
  const page = pages[0] || await browser.newPage();

  try {
    switch (action) {
      case 'navigate':
        await page.goto(params.url, { 
          waitUntil: params.wait_until || 'load', 
          timeout: params.timeout || 30000 
        });
        console.log(`Navigated to ${page.url()}`);
        break;

      case 'click':
        await page.waitForSelector(params.selector, { timeout: params.timeout || 10000 });
        await page.click(params.selector);
        console.log(`Clicked ${params.selector}`);
        break;

      case 'type':
        await page.waitForSelector(params.selector, { timeout: params.timeout || 10000 });
        await page.type(params.selector, params.text);
        console.log(`Typed into ${params.selector}`);
        break;

      case 'screenshot':
        const shotPath = params.path || '/sdcard/browser_shot.png';
        await page.screenshot({ path: shotPath, fullPage: true });
        console.log(`Screenshot saved to ${shotPath}`);
        break;

      case 'evaluate':
        const result = await page.evaluate(params.script);
        console.log(JSON.stringify(result));
        break;

      case 'content':
        const content = await page.content();
        console.log(content);
        break;

      case 'wait_for':
        await page.waitForSelector(params.selector, { timeout: params.timeout || 30000 });
        console.log(`Selector ${params.selector} is now present`);
        break;

      default:
        console.error(`Unknown action: ${action}`);
        process.exit(1);
    }
  } catch (e) {
    console.error(`Error during ${action}: ${e.message}`);
    process.exit(1);
  } finally {
    await browser.close();
  }
}

run();
