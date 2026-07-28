const debuggerURL = process.env.PULSE_CDP_URL || 'http://127.0.0.1:9223'
const appURL = process.env.PULSE_E2E_URL || 'http://localhost:8080/'
const runID = Date.now()
const sourceName = `Pulse E2E RSS ${runID}`
const feedURL = `${process.env.PULSE_E2E_FEED || 'https://www.rssboard.org/files/sample-rss-2.xml'}?pulse_e2e=${runID}`

const pages = await fetch(`${debuggerURL}/json/list`).then((response) => response.json())
const page = pages.find((candidate) => candidate.type === 'page')
if (!page) throw new Error('Chrome DevTools page target was not found')

const socket = new WebSocket(page.webSocketDebuggerUrl)
await new Promise((resolve, reject) => {
  socket.addEventListener('open', resolve, { once: true })
  socket.addEventListener('error', reject, { once: true })
})

let nextID = 1
const pending = new Map()
socket.addEventListener('message', (event) => {
  const message = JSON.parse(event.data)
  if (!message.id || !pending.has(message.id)) return
  const { resolve, reject } = pending.get(message.id)
  pending.delete(message.id)
  if (message.error) reject(new Error(message.error.message))
  else resolve(message.result)
})

function command(method, params = {}) {
  const id = nextID++
  socket.send(JSON.stringify({ id, method, params }))
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }))
}

async function evaluate(expression) {
  const result = await command('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
  })
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.exception?.description || 'browser evaluation failed')
  }
  return result.result.value
}

async function waitFor(expression, description, timeout = 30000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await evaluate(expression)) return
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  const text = await evaluate('document.body.innerText')
  throw new Error(`Timed out waiting for ${description}\n${text}`)
}

await command('Page.enable')
await command('Runtime.enable')
await command('Page.navigate', { url: appURL })
await waitFor(`document.body.innerText.includes('添加信息源')`, 'Pulse source page')

await evaluate(`Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('添加信息源')).click()`)
await waitFor(`document.querySelector('[role="dialog"]') !== null`, 'source dialog')

await command('Emulation.setDeviceMetricsOverride', {
  width: 390,
  height: 480,
  deviceScaleFactor: 1,
  mobile: true,
})
const mobileDialog = await evaluate(`(() => {
  const dialog = document.querySelector('[role="dialog"]')
  const action = Array.from(dialog.querySelectorAll('button')).find((button) => button.textContent.includes('测试并预览'))
  dialog.scrollTop = dialog.scrollHeight
  const dialogRect = dialog.getBoundingClientRect()
  const actionRect = action.getBoundingClientRect()
  return {
    scrollable: dialog.scrollHeight > dialog.clientHeight,
    contained: dialogRect.top >= 0 && dialogRect.bottom <= window.innerHeight,
    actionVisible: actionRect.top >= 0 && actionRect.bottom <= window.innerHeight,
  }
})()`)
if (!mobileDialog.scrollable || !mobileDialog.contained || !mobileDialog.actionVisible) {
  throw new Error(`Mobile dialog layout assertion failed: ${JSON.stringify(mobileDialog)}`)
}

await evaluate(`(() => {
  const set = (selector, value) => {
    const input = document.querySelector(selector)
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
    setter.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  }
  set('input[placeholder="例如：技术博客"]', ${JSON.stringify(sourceName)})
  set('input[type="url"]', ${JSON.stringify(feedURL)})
})()`)
await evaluate(`Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('测试并预览')).click()`)
await waitFor(`document.body.innerText.includes('连接成功')`, 'RSS preview', 60000)
await evaluate(`Array.from(document.querySelectorAll('button')).find((button) => button.textContent.includes('保存并启用')).click()`)
await waitFor(`document.body.innerText.includes(${JSON.stringify(sourceName)}) && !document.querySelector('[role="dialog"]')`, 'saved RSS source')

const result = await evaluate(`({
  url: location.href,
  sourceVisible: document.body.innerText.includes(${JSON.stringify(sourceName)}),
  successVisible: document.body.innerText.includes('已添加'),
})`)
socket.close()
if (!result.sourceVisible || !result.successVisible) {
  throw new Error(`RSS journey assertion failed: ${JSON.stringify(result)}`)
}
process.stdout.write(`RSS browser journey passed: ${sourceName}\n`)
