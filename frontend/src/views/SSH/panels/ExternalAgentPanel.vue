<template>
  <div class="external-agent-panel">
    <Message ref="messageRef" />

    <main class="panel-content">
      <section v-if="records.length === 0" class="empty-state">
        <div class="empty-mark">
          <svg viewBox="0 0 24 24"><path d="M15.5 7.5a4 4 0 1 1-7.2 2.4A4 4 0 0 1 15.5 7.5Z"/><path d="m13.8 13.2 7.2 7.3M18 17l-2 2M9 14l-6 6"/></svg>
        </div>
        <span class="eyebrow">尚未登记外部 Agent</span>
        <h3>先把接管规则交给主力机上的 Agent</h3>
        <p class="empty-copy">这段提示词要求它使用独立密钥、只短暂使用 root，并在服务安装后迁移到专用账号。提示词不包含本机地址、密码或任何密钥正文。</p>

        <div class="prompt-card">
          <div class="prompt-head">
            <span>外部 Agent 接管协议</span>
            <button @click="copyPrompt(null)">复制提示词</button>
          </div>
          <pre>{{ buildPrompt(null) }}</pre>
        </div>

        <div class="empty-actions">
          <button class="primary-button" @click="showRegister = true">登记一个外部 Agent</button>
          <button class="secondary-button" :disabled="scanning" @click="scanAudit">扫描已有密钥</button>
        </div>
      </section>

      <template v-else>
        <section class="health-strip" :class="`tone-${overallStatus.tone}`">
          <span class="health-dot"></span>
          <div class="health-copy">
            <strong>{{ overallStatus.title }}</strong>
            <small>{{ overallStatus.detail }}</small>
          </div>
          <button class="icon-button compact" :disabled="scanning" title="立即扫描" @click="scanAudit">
            <svg :class="{ spinning: scanning }" viewBox="0 0 24 24"><path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7"/></svg>
          </button>
          <button class="advanced-toggle" @click="showAdvanced = !showAdvanced">
            {{ showAdvanced ? '收起' : '高级信息' }}
            <svg :class="{ open: showAdvanced }" viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></svg>
          </button>
        </section>

        <div v-if="showAdvanced" class="advanced-section">
          <div v-if="audit && !audit.canInspectAll" class="scope-warning">
            当前连接不是 root，只能检查当前账号的 authorized_keys；无法确认其他用户是否仍保留 Agent 密钥。
          </div>

          <section v-if="rootSessionViews.length" class="root-session-card">
            <div class="section-heading"><strong>当前 root 登录归属</strong><span>{{ rootSessionViews.length }}</span></div>
            <p>根据 SSH 认证日志中的登录方式和公钥指纹判断。</p>
            <div v-for="session in rootSessionViews" :key="`${session.terminal}-${session.source}`" class="root-session-row">
              <span class="owner-badge" :class="session.ownerType">{{ session.ownerLabel }}</span>
              <span>{{ session.terminal }}</span>
              <span>{{ session.authMethodLabel }}</span>
              <span v-if="session.source">来源 {{ maskSource(session.source) }}</span>
            </div>
          </section>

          <div v-if="!rootSessionViews.length" class="advanced-empty">当前没有检测到 root SSH 会话。</div>
        </div>

        <div class="list-heading"><strong>外部 Agent</strong><span>{{ records.length }}</span></div>

        <section class="agent-list">
          <article v-for="record in recordViews" :key="record.id" class="agent-card" :class="`state-${record.state}`">
            <button class="collapse-head" @click="toggleExpanded(record.id)">
              <span class="state-dot"></span>
              <span class="agent-heading">
                <strong>{{ record.name }}</strong>
                <small>{{ record.serviceUser }} · {{ record.keyLabel }}</small>
              </span>
              <span class="state-badge">{{ stateLabel(record.state) }}</span>
              <span class="detail-label">详情</span>
              <svg class="chevron" :class="{ open: isExpanded(record.id) }" viewBox="0 0 24 24"><path d="m6 9 6 6 6-6"/></svg>
            </button>

            <div v-if="isExpanded(record.id)" class="agent-body">
              <div class="lifecycle">
                <div :class="{ done: record.matchingKeys.length > 0 }"><i>1</i><span>独立密钥</span></div>
                <div :class="{ done: record.rootKeys.length > 0, danger: record.rootKeys.length > 0 }"><i>2</i><span>临时 root</span></div>
                <div :class="{ done: record.serviceKeys.length > 0 }"><i>3</i><span>专用用户</span></div>
                <div :class="{ done: record.serviceKeys.length > 0 && record.rootKeys.length === 0 }"><i>4</i><span>root 已撤销</span></div>
              </div>

              <dl class="agent-meta">
                <div><dt>目标专用用户</dt><dd>{{ record.serviceUser }}</dd></div>
                <div><dt>Agent 会话状态</dt><dd>{{ record.sessions.length ? `当前在线（${record.sessions.length}）` : (record.ambiguousSessions.length ? '有同名会话，但身份未确认' : (record.recentLogins.length ? '近期登录过，当前已离线' : '当前未确认在线')) }}</dd></div>
                <div><dt>监管边界</dt><dd>SSH 登录与密钥；不读取外部 Agent 的思考过程</dd></div>
              </dl>

              <div v-if="record.matchingKeys.length" class="key-list">
                <div v-for="key in record.matchingKeys" :key="`${key.username}-${key.fingerprint}`" class="key-row">
                  <div class="key-main">
                    <span class="user-pill" :class="{ root: key.isRoot }">{{ key.username }}</span>
                    <code>{{ shortFingerprint(key.fingerprint) }}</code>
                    <small>{{ key.algorithm }}</small>
                  </div>
                  <div class="key-actions">
                    <button class="cleanup-button" @click="openCleanup(record)">工作机清理</button>
                    <button class="danger-button" :disabled="!canRevokeKey(key)" :title="canRevokeKey(key) ? '撤销这把密钥' : `当前账号无权撤销 ${key.username} 的密钥`" @click="revokeKey(record, key)">服务器撤销</button>
                  </div>
                </div>
              </div>
              <div v-else class="waiting-box">尚未在服务器发现标签为 <code>{{ record.keyLabel }}</code> 的授权密钥。</div>

              <div v-if="record.sessions.length" class="session-list">
                <div v-for="session in record.sessions" :key="`${session.username}-${session.terminal}`" class="session-row">
                  <span class="live-dot"></span>
                  <strong>{{ session.username }}</strong>
                  <span class="auth-pill">Agent 指纹匹配</span>
                  <span>{{ session.terminal }}</span>
                  <span>{{ session.loginAt }}</span>
                  <span v-if="session.source">来源 {{ maskSource(session.source) }}</span>
                </div>
              </div>
              <div v-else-if="record.recentLogins.length" class="recent-login-row">
                <span class="recent-dot"></span>
                <strong>最近指纹匹配登录</strong>
                <span>{{ record.recentLogins[0].loginAt || '24 小时内' }}</span>
                <span v-if="record.recentLogins[0].source">来源 {{ maskSource(record.recentLogins[0].source) }}</span>
                <span>当前连接已结束</span>
              </div>

              <div class="card-actions">
                <button @click="copyPrompt(record)">复制该 Agent 的接管提示词</button>
                <button @click="removeRecord(record)">移除监管记录</button>
              </div>
            </div>
          </article>
        </section>

        <button class="add-agent-button" @click="showRegister = true">+ 登记外部 Agent</button>
      </template>

      <section v-if="unregisteredKeys.length" class="discovered-card">
        <div class="section-heading"><strong>服务器上未登记的授权密钥</strong><span>{{ unregisteredKeys.length }}</span></div>
        <p>这里只展示标签和指纹，不展示公钥正文。普通人工密钥也可能出现在这里，请自行确认归属。</p>
        <div v-for="key in unregisteredKeys" :key="`${key.username}-${key.fingerprint}`" class="key-row">
          <div class="key-main">
            <span class="user-pill" :class="{ root: key.isRoot }">{{ key.username }}</span>
            <code>{{ shortFingerprint(key.fingerprint) }}</code>
            <small>{{ key.comment || '无标签' }}</small>
            <span v-if="activeSessionsForKey(key).length" class="discovery-activity live">当前在线</span>
            <span v-else-if="recentSessionsForKey(key).length" class="discovery-activity recent">近期登录</span>
          </div>
          <button class="register-button" @click="registerDiscoveredKey(key)">一键纳管</button>
        </div>
      </section>
    </main>

    <Teleport to="body">
      <div v-if="showRegister" class="modal-mask" @click.self="showRegister = false">
        <form class="register-modal" @submit.prevent="registerAgent">
          <div class="modal-head"><div><h3>登记外部 Agent</h3><p>这里只登记标签和专用用户名，不保存任何密钥。</p></div><button type="button" @click="showRegister=false">×</button></div>
          <label>Agent 名称<input v-model.trim="form.name" required maxlength="40" placeholder="例如：主力机 Codex" /></label>
          <label>公钥管理标签<input v-model.trim="form.keyLabel" required maxlength="64" pattern="[A-Za-z0-9._-]+" /></label>
          <label>最终专用用户名<input v-model.trim="form.serviceUser" required maxlength="32" pattern="[a-z_][a-z0-9_-]*" placeholder="例如：deploy-agent" /></label>
          <label>工作机私钥路径（可选）<input v-model.trim="form.workstationKeyPath" maxlength="240" placeholder="例如：~/.ssh/codex-deploy；不保存私钥内容" /></label>
          <div class="register-note">root 只用于首次安装服务。面板会持续把“密钥仍留在 root”标记为高风险，直到它迁移到专用用户并撤销。</div>
          <div class="modal-actions"><button type="button" @click="showRegister=false">取消</button><button class="primary-button" type="submit">登记并生成提示词</button></div>
        </form>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="cleanupRecord" class="modal-mask" @click.self="cleanupRecord = null">
        <div class="register-modal cleanup-modal">
          <div class="modal-head"><div><h3>清理工作机私钥</h3><p>以下命令必须在保存私钥的主力机上执行，不是在服务器上执行。</p></div><button type="button" @click="cleanupRecord=null">×</button></div>
          <label>确认工作机私钥路径<input v-model.trim="cleanupPath" placeholder="~/.ssh/外部-agent-密钥" /></label>
          <div class="cleanup-warning">先核对公钥指纹，再从 ssh-agent 卸载并交互式删除。路径为空时命令会保留醒目的占位符，不会假装知道私钥在哪里。</div>
          <pre class="command-preview">{{ workstationCleanupCommand(cleanupPath) }}</pre>
          <div class="modal-actions"><button type="button" @click="cleanupRecord=null">稍后处理</button><button class="primary-button" type="button" @click="copyCleanupCommand">复制清理命令</button></div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, onUnmounted, reactive, ref } from 'vue'
import { GetConnection, GetExternalAgentAudit, GetExternalAgentScopeID, RevokeExternalAgentKey } from '../../../../bindings/changeme/ssh/sshservice.js'
import Message from '../../../components/Message.vue'
import { useConfirm } from '../../../utils/confirm'
import { useExternalAgentStore } from '../../../stores/externalAgents'

const connId = inject('connId')
const recordScope = ref('')
const store = useExternalAgentStore()
const { confirm } = useConfirm()
const messageRef = ref(null)
const audit = ref(null)
const scanning = ref(false)
const showRegister = ref(false)
const cleanupRecord = ref(null)
const cleanupPath = ref('')
const connectionUsername = ref('')
const expanded = ref({})
const showAdvanced = ref(false)
const form = reactive({ name: '', keyLabel: `qssh-agent-${Math.random().toString(36).slice(2, 8)}`, serviceUser: 'deploy-agent', workstationKeyPath: '' })

const records = computed(() => recordScope.value ? store.getRecords(recordScope.value) : [])
const allKeys = computed(() => audit.value?.keys || [])
const allSessions = computed(() => audit.value?.sessions || [])

const recordViews = computed(() => records.value.map(record => {
	const matchingKeys = allKeys.value.filter(key =>
		record.fingerprint ? key.fingerprint === record.fingerprint : key.comment === record.keyLabel
	)
  const rootKeys = matchingKeys.filter(key => key.username === 'root')
  const serviceKeys = matchingKeys.filter(key => key.username === record.serviceUser)
  const managedFingerprints = new Set(matchingKeys.map(key => key.fingerprint))
  const sessions = allSessions.value.filter(session => session.active && session.fingerprint && managedFingerprints.has(session.fingerprint))
  const recentLogins = allSessions.value.filter(session => !session.active && session.fingerprint && managedFingerprints.has(session.fingerprint))
  const ambiguousSessions = allSessions.value.filter(session =>
    session.active && !session.fingerprint && (session.username === record.serviceUser || (rootKeys.length && session.username === 'root'))
  )
  let state = audit.value ? 'waiting' : 'unknown'
  if (rootKeys.length && !serviceKeys.length) state = 'root'
  if (rootKeys.length && serviceKeys.length) state = 'migrating'
  if (!rootKeys.length && serviceKeys.length) state = sessions.length ? 'active' : 'ready'
  return { ...record, matchingKeys, rootKeys, serviceKeys, sessions, recentLogins, ambiguousSessions, state }
}))

const rootKeyCount = computed(() => recordViews.value.reduce((sum, record) => sum + record.rootKeys.length, 0))
const activeManagedSessions = computed(() => recordViews.value.flatMap(record => record.sessions))
const registeredLabels = computed(() => new Set(records.value.map(record => record.keyLabel)))
const registeredFingerprints = computed(() => new Set(records.value.map(record => record.fingerprint).filter(Boolean)))
const unregisteredKeys = computed(() => allKeys.value.filter(key =>
  !registeredLabels.value.has(key.comment) && !registeredFingerprints.value.has(key.fingerprint)
))
const rootSessionViews = computed(() => allSessions.value.filter(session => session.active && session.username === 'root').map(session => {
  const matchedRecord = recordViews.value.find(record =>
    session.fingerprint && record.matchingKeys.some(key => key.fingerprint === session.fingerprint)
  )
  if (matchedRecord) {
    return { ...session, ownerType: 'agent', ownerLabel: `${matchedRecord.name} · 临时 root`, authMethodLabel: '已登记密钥' }
  }
  if (session.authMethod === 'password' || session.authMethod?.startsWith('keyboard-interactive')) {
    return { ...session, ownerType: 'user', ownerLabel: '用户 root 登录', authMethodLabel: '密码认证' }
  }
  if (session.fingerprint) {
    return { ...session, ownerType: 'other', ownerLabel: '其他密钥登录', authMethodLabel: shortFingerprint(session.fingerprint) }
  }
  return { ...session, ownerType: 'unknown', ownerLabel: '身份未确认', authMethodLabel: '缺少认证日志' }
}))

const overallStatus = computed(() => {
	if (!audit.value) return { tone: 'neutral', title: '正在确认服务器状态', detail: `${records.value.length} 个 Agent 已登记` }
	if (!audit.value.canInspectAll) return { tone: 'neutral', title: '监管范围受限', detail: '当前不是 root 连接，无法确认其他账号或 root 的 Agent 密钥' }
	if (rootKeyCount.value > 0) return { tone: 'danger', title: `${rootKeyCount.value} 把 Agent 密钥仍可登录 root`, detail: '需要迁移到专用用户并撤销 root 授权' }
  if (activeManagedSessions.value.length > 0) return { tone: 'active', title: `${activeManagedSessions.value.length} 个外部 Agent 当前在线`, detail: `${records.value.length} 个已登记 · 身份已通过密钥指纹确认` }
  return { tone: 'safe', title: '外部 Agent 权限正常', detail: `${records.value.length} 个已登记 · 当前没有确认在线的 Agent` }
})

function stateLabel(state) {
  return { unknown: '尚未扫描', waiting: '等待接入', root: '临时 root', migrating: '迁移未完成', ready: '权限已收敛', active: 'SSH 会话在线' }[state] || state
}

function toggleExpanded(id) { expanded.value[id] = !isExpanded(id) }
function isExpanded(id) { return expanded.value[id] === true }
function shortFingerprint(value) { return value?.length > 26 ? `${value.slice(0, 18)}…${value.slice(-7)}` : value }
function canRevokeKey(key) { return connectionUsername.value === 'root' || connectionUsername.value === key.username }

async function legacyScopeID(connection) {
  if (!connection?.host || !connection?.username) return ''
  const raw = `${connection.host.trim().toLowerCase()}:${connection.port}:${connection.username}`
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(raw))
  const prefix = Array.from(new Uint8Array(digest).slice(0, 12), value => value.toString(16).padStart(2, '0')).join('')
  return `external-agent-scope-${prefix}`
}

function maskSource(value) {
  if (!value) return ''
  if (value.includes(':')) return value.replace(/:[^:]+$/, ':…')
  const parts = value.split('.')
  return parts.length === 4 ? `${parts[0]}.${parts[1]}.*.*` : value
}
function activeSessionsForKey(key) {
  return allSessions.value.filter(session => session.active && session.fingerprint === key.fingerprint)
}
function recentSessionsForKey(key) {
  return allSessions.value.filter(session => !session.active && session.fingerprint === key.fingerprint)
}

function buildPrompt(record) {
  const label = record?.keyLabel || '<由舟SSH登记后提供的 KEY_LABEL>'
  const serviceUser = record?.serviceUser || '<专用部署用户名，例如 deploy-agent>'
  return `你将从一台外部主力机通过 SSH 接管一台由“舟SSH”监管的服务器。你的任务是把接管流程完整做完，并把用户必须亲自执行的步骤写成可直接复制的命令。不要让用户自己猜命令，也不要在正常流程中每一步都反复询问。

固定身份与安全边界
- 为本次接管生成独立 Ed25519 密钥，管理标签必须是：${label}
- 最终长期使用的专用用户名必须是：${serviceUser}
- 私钥只能保存在当前工作机，不得输出、上传、粘贴进聊天或写入远端
- 远端只安装公钥；不得索取、记录或保存服务器密码
- 不得启用 SSH Agent Forwarding，不得复用个人默认密钥，不得使用 StrictHostKeyChecking=no
- root 仅用于首次部署和权限迁移；不得作为 Agent 的长期工作账号

阶段一：在工作机准备独立密钥
1. 选择清晰的独立私钥路径，先检查该路径是否已存在；如果存在，停止并更换路径，绝不覆盖旧密钥。
2. 使用 ssh-keygen -t ed25519 生成密钥，注释必须是 ${label}。
3. 只向用户报告：私钥路径、公钥路径、完整 SHA-256 指纹、管理标签和完整公钥一行。绝不显示私钥正文。

阶段二：明确告诉用户如何临时安装 root 公钥
1. 告诉用户先使用自己的认证方式登录服务器，例如：ssh root@<SERVER_HOST>。
2. 将下面命令中的 <PUBLIC_KEY_LINE> 替换为刚生成的完整公钥一行，然后把替换后的完整命令块交给用户。不得保留占位符，不得让用户自行拼接：

install -d -m 700 /root/.ssh
KEY_LINE='<PUBLIC_KEY_LINE>'
touch /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
grep -qxF "$KEY_LINE" /root/.ssh/authorized_keys || printf '%s\\n' "$KEY_LINE" >> /root/.ssh/authorized_keys
ssh-keygen -lf /root/.ssh/authorized_keys

3. 解释最后一条命令用于让用户核对已安装密钥的 SHA-256 指纹。
4. 在这里暂停，只等待用户回复“已安装”。这是正常流程中唯一必需的用户交接点；在用户确认前不得尝试连接。

阶段三：Agent 使用临时 root 完成部署
1. 用户确认后，使用独立密钥和 IdentitiesOnly=yes 连接 root；首次出现服务器主机指纹时，要求用户核对，不得绕过主机身份检查。
2. 连接后先用 id、hostname 等只读命令确认当前服务器和账号，再开始部署。
3. 一次性说明本阶段准备安装的软件、修改的文件、开放的端口、创建的服务和回滚方法，然后连续完成已约定的部署任务；无需对每条普通命令重复索取确认。
4. 每条命令执行后记录退出状态和实际变化。如果发现目标服务器不符、需要删除用户数据、修改 SSH 全局配置、扩大防火墙暴露面或执行计划外高风险操作，立即停止并单独询问用户。

阶段四：创建专用用户并迁移密钥
1. 创建或确认专用用户 ${serviceUser}，默认不给密码登录能力，不授予完整免密 sudo。
2. 只授予任务真正需要的最小目录权限、服务权限或精确 sudo 命令；说明每项权限的用途。
3. 将同一公钥写入 ${serviceUser} 的 authorized_keys，正确设置主目录、.ssh 目录和文件的所有者及 700/600 权限；操作必须可重复执行且不能产生重复公钥。
4. 保留当前 root 会话，同时从一个新的独立 SSH 会话使用该密钥登录 ${serviceUser}，执行 id 和任务所需的最小验证，证明专用账号确实可用。

阶段五：验证成功后收回 root
1. 只有专用用户的新会话验证成功后，才能处理 root 授权；验证失败时保留 root 公钥并先修复问题。
2. 先备份 /root/.ssh/authorized_keys，再按完整 SHA-256 指纹精确删除本 Agent 的 root 公钥，不得只按模糊文字删除，也不得影响其他密钥。
3. 确认 root 的 authorized_keys 中已不存在该指纹，确认专用用户仍可登录，然后退出临时 root 会话。
4. 后续所有正常任务只使用 ${serviceUser}。若以后确实需要 root，先说明原因和范围，再让用户重新授权。

阶段六：交付与将来撤销
1. 完成后报告：服务器、专用用户名、公钥指纹、密钥标签、root 授权是否已撤销、专用用户权限、服务状态、修改内容、回滚方法和遗留风险。
2. 向用户说明：只要还需要该 Agent 接管服务器，就必须保留工作机私钥；不要在正常部署完成后删除它。
3. 同时给出“彻底撤销 Agent”时的两段操作：先在服务器删除专用用户 authorized_keys 中的对应指纹，再在工作机用 ssh-add -d 卸载并用 rm -i 删除私钥和 .pub 文件。必须带入真实用户名、指纹和已确认的私钥路径，不得留下含糊占位符。

操作记录
- 按实际发生顺序维护清单，每一步标记为“用户”或“Agent”。用户安装 root 公钥属于用户步骤，其余由 Agent 实际执行的操作属于 Agent 步骤。
- 最终按步骤数量计算：用户占比 = 用户步骤数 / 总步骤数，Agent 占比 = Agent 步骤数 / 总步骤数。
- 明确说明这是操作步骤占比，不代表耗时、命令数量或工作量。

现在先执行阶段一，然后输出密钥信息和阶段二中已经替换好公钥的用户命令块，并停下来等待用户回复“已安装”。`
}

async function copyPrompt(record) {
  try {
    await navigator.clipboard.writeText(buildPrompt(record))
    messageRef.value?.success('接管提示词已复制')
  } catch (error) {
    messageRef.value?.error('复制失败，请手动选择文本')
  }
}

function registerAgent() {
  if (!recordScope.value) return
  store.addRecord(recordScope.value, form)
  const added = store.getRecords(recordScope.value).at(-1)
  expanded.value[added.id] = true
  showRegister.value = false
  copyPrompt(added)
  scanAudit(true)
  ensureAuditTimer()
  form.name = ''
  form.keyLabel = `qssh-agent-${Math.random().toString(36).slice(2, 8)}`
  form.workstationKeyPath = ''
}

function registerDiscoveredKey(key) {
	if (!recordScope.value) return
	const keyLabel = `qssh-key-${key.fingerprint.slice(-12).replace(/[^A-Za-z0-9]/g, '')}`
	const agentName = `${key.username} 外部 Agent`
  const serviceUser = key.isRoot ? 'deploy-agent' : key.username
  store.addRecord(recordScope.value, {
    name: agentName,
    keyLabel,
    serviceUser,
    fingerprint: key.fingerprint,
    workstationKeyPath: ''
  })
  const added = store.getRecords(recordScope.value).at(-1)
  expanded.value[added.id] = true
  if (key.isRoot) {
    messageRef.value?.info(`已纳入监管：该密钥仍在 root，请迁移到专用用户 ${serviceUser}`)
  } else {
    messageRef.value?.success(`已将 ${key.username} / ${key.comment || shortFingerprint(key.fingerprint)} 纳入监管`)
  }
  scanAudit(true)
  ensureAuditTimer()
}

async function scanAudit(silent = false) {
  if (scanning.value) return
  scanning.value = true
  try {
    audit.value = await GetExternalAgentAudit(connId)
    if (!silent) {
      const activeCount = audit.value?.sessions?.filter(session => session.active).length || 0
      messageRef.value?.success(`扫描完成：${audit.value?.keys?.length || 0} 个授权密钥，${activeCount} 个当前 SSH 会话`)
    }
  } catch (error) {
    if (!silent) messageRef.value?.error('扫描失败：' + (error?.message || error))
  } finally {
    scanning.value = false
  }
}

async function revokeKey(record, key) {
  const ok = await confirm({
    title: key.isRoot ? '撤销 Agent 的 root 权限' : '撤销 Agent 密钥',
    message: `将从 ${key.username} 的 authorized_keys 删除“${record.keyLabel}”。\n\n删除前会创建 authorized_keys.qssh.bak 备份。此操作会使对应私钥失去该账号的登录权限。`,
    danger: true
  })
  if (!ok) return
	try {
		await RevokeExternalAgentKey(connId, key.username, key.fingerprint)
		messageRef.value?.success(`已撤销 ${key.username} 的 Agent 密钥`)
		await scanAudit()
		if (!audit.value?.keys?.some(item => item.fingerprint === key.fingerprint)) openCleanup(record)
  } catch (error) {
    messageRef.value?.error('撤销失败：' + (error?.message || error))
  }
}

function openCleanup(record) {
  cleanupRecord.value = record
  cleanupPath.value = record.workstationKeyPath || ''
}

function shellSingleQuote(value) {
  return `'${value.replace(/'/g, `'"'"'`)}'`
}

function workstationCleanupCommand(path) {
  const value = path || '<WORKSTATION_PRIVATE_KEY_PATH>'
  return `# 仅在保存私钥的工作机上执行；不要在服务器上执行
KEY_PATH=${shellSingleQuote(value)}
case "\${KEY_PATH}" in "~/"*) KEY_PATH="\${HOME}/\${KEY_PATH#~/}" ;; esac

# 1. 删除前核对公钥指纹
ssh-keygen -lf "\${KEY_PATH}.pub"

# 2. 从本机 ssh-agent 卸载
ssh-add -d "\${KEY_PATH}" 2>/dev/null || true

# 3. 交互式删除私钥与公钥；确认路径无误后再输入 y
rm -i -- "\${KEY_PATH}" "\${KEY_PATH}.pub"`
}

async function copyCleanupCommand() {
  try {
    await navigator.clipboard.writeText(workstationCleanupCommand(cleanupPath.value))
    messageRef.value?.success('工作机清理命令已复制')
    cleanupRecord.value = null
  } catch {
    messageRef.value?.error('复制失败，请手动选择命令')
  }
}

async function removeRecord(record) {
  const ok = await confirm({ title: '移除监管记录', message: `只移除“${record.name}”的本地监管记录，不会删除服务器上的密钥。`, danger: false })
  if (ok) {
    store.removeRecord(recordScope.value, record.id)
    if (!records.value.length && auditTimer) {
      window.clearInterval(auditTimer)
      auditTimer = null
    }
  }
}

let auditTimer = null
let panelUnmounted = false

function ensureAuditTimer() {
	if (auditTimer || !records.value.length) return
	auditTimer = window.setInterval(() => {
		if (document.visibilityState === 'visible' && records.value.length) scanAudit(true)
	}, 30000)
}

onMounted(async () => {
  try {
    const connection = await GetConnection(connId)
    connectionUsername.value = connection?.username || ''
    recordScope.value = await GetExternalAgentScopeID(connId)
		const legacyScope = await legacyScopeID(connection)
		store.migrateRecords(legacyScope, recordScope.value)
  } catch {
		// 老版本或异常连接回退为本次会话 ID；仍不写入服务器地址。
		recordScope.value = connId
	}
	if (panelUnmounted) return
	if (records.value.length) {
		scanAudit()
		ensureAuditTimer()
	}
})

onUnmounted(() => {
	panelUnmounted = true
	if (auditTimer) window.clearInterval(auditTimer)
})
</script>

<style scoped>
.external-agent-panel { height:100%; display:flex; flex-direction:column; background:var(--toolbar-4); color:var(--text-primary); min-width:480px }
.key-main,.card-actions,.empty-actions,.modal-actions,.section-heading { display:flex; align-items:center }
svg { width:18px; height:18px; fill:none; stroke:currentColor; stroke-width:1.8; stroke-linecap:round; stroke-linejoin:round }
button { font:inherit }.icon-button { width:32px; height:32px; display:grid; place-items:center; border:1px solid var(--border-default); background:var(--surface-1); color:var(--text-secondary); border-radius:7px; cursor:pointer }.icon-button:hover { color:var(--text-primary); background:var(--surface-hover) }.icon-button:disabled { opacity:.5; cursor:default }.spinning { animation:spin .8s linear infinite }
.icon-button.compact { width:26px; height:26px; flex:0 0 auto; border:0; background:transparent }.icon-button.compact svg { width:14px; height:14px }
.panel-content { flex:1; overflow:auto; padding:18px }.empty-state { max-width:760px; min-height:calc(100% - 36px); margin:0 auto; display:flex; flex-direction:column; justify-content:center; align-items:center; text-align:center }.empty-mark { width:56px; height:56px; display:grid; place-items:center; border-radius:16px; color:var(--accent-purple); background:var(--accent-purple-bg); border:1px solid var(--border-purple); margin-bottom:14px }.empty-mark svg { width:28px; height:28px }.eyebrow { color:var(--accent-purple); font-size:10px; letter-spacing:.12em; font-weight:700 }.empty-state h3 { margin:7px 0; font-size:20px }.empty-copy { max-width:620px; margin:0 0 16px; color:var(--text-secondary); font-size:12px; line-height:1.7 }
.prompt-card { width:100%; text-align:left; border:1px solid var(--border-default); border-radius:10px; overflow:hidden; background:var(--card-bg); box-shadow:var(--shadow-sm) }.prompt-head { height:38px; display:flex; align-items:center; justify-content:space-between; padding:0 12px; border-bottom:1px solid var(--border-default); background:var(--card-header-bg); font-size:11px; font-weight:600 }.prompt-head button,.card-actions button { border:0; background:none; color:var(--accent-primary); cursor:pointer }.prompt-card pre { max-height:260px; overflow:auto; white-space:pre-wrap; margin:0; padding:14px; font:11px/1.65 ui-monospace,SFMono-Regular,Menlo,monospace; color:var(--text-secondary); user-select:text }.empty-actions { gap:8px; margin-top:14px }
.primary-button,.secondary-button,.add-agent-button { border-radius:7px; padding:7px 12px; cursor:pointer }.primary-button { border:1px solid var(--accent-primary); background:var(--accent-primary); color:var(--text-on-accent) }.secondary-button,.add-agent-button { border:1px solid var(--border-default); background:var(--surface-1); color:var(--text-primary) }.register-button { flex:0 0 auto; padding:5px 9px; border:1px solid var(--border-accent); border-radius:5px; background:var(--primary-bg); color:var(--accent-primary); font-size:9px; cursor:pointer }.register-button:hover { background:var(--primary-bg-hover) }.key-actions { display:flex; gap:5px }.cleanup-button { border:1px solid var(--border-default); background:var(--surface-2); color:var(--text-secondary); border-radius:5px; padding:4px 7px; font-size:9px; cursor:pointer }.cleanup-button:hover { color:var(--text-primary); background:var(--surface-hover) }.danger-button:disabled { opacity:.45; cursor:not-allowed }
.discovery-activity { padding:2px 6px; border-radius:999px; font-size:9px }.discovery-activity.live { color:var(--accent-success); background:var(--success-bg) }.discovery-activity.recent { color:var(--accent-warning); background:var(--warning-bg) }
.health-strip { min-height:58px; display:flex; align-items:center; gap:11px; margin-bottom:16px; padding:10px 12px; border:1px solid var(--border-default); border-radius:9px; background:var(--card-bg) }.health-dot { width:9px; height:9px; flex:0 0 auto; border-radius:50%; background:var(--text-muted) }.health-copy { flex:1; min-width:0; display:flex; flex-direction:column; gap:3px }.health-copy strong { font-size:12px }.health-copy small { color:var(--text-secondary); font-size:9px }.tone-danger { border-color:var(--border-danger); background:var(--danger-bg) }.tone-danger .health-dot { background:var(--accent-danger) }.tone-active { border-color:var(--border-success) }.tone-active .health-dot,.tone-safe .health-dot { background:var(--accent-success) }.advanced-toggle { display:flex; align-items:center; gap:4px; padding:5px 7px; border:0; background:none; color:var(--text-secondary); font-size:9px; cursor:pointer }.advanced-toggle:hover { color:var(--text-primary) }.advanced-toggle svg { width:12px; transition:transform .15s }.advanced-toggle svg.open { transform:rotate(180deg) }.advanced-section { margin:-7px 0 16px; padding:10px; border:1px solid var(--border-default); border-radius:8px; background:var(--surface-1) }.advanced-section .scope-warning,.advanced-section .root-session-card { margin-bottom:0 }.advanced-section .scope-warning + .root-session-card { margin-top:8px }.advanced-empty { color:var(--text-muted); font-size:9px }.list-heading { display:flex; align-items:center; gap:6px; margin:0 2px 8px }.list-heading strong { font-size:11px }.list-heading span { padding:1px 5px; border-radius:999px; color:var(--text-secondary); background:var(--surface-2); font-size:9px }
.scope-warning,.waiting-box,.register-note { padding:9px 11px; border-radius:7px; font-size:11px; line-height:1.5 }.scope-warning { margin-bottom:12px; color:var(--accent-warning); border:1px solid var(--border-warning); background:var(--warning-bg) }.agent-list { display:flex; flex-direction:column; gap:9px }.agent-card { border:1px solid var(--border-default); border-radius:9px; overflow:hidden; background:var(--card-bg) }.agent-card.state-root,.agent-card.state-migrating { border-color:var(--border-danger) }.agent-card.state-active { border-color:var(--border-success) }
.root-session-card { margin-bottom:12px; padding:11px; border:1px solid var(--border-default); border-radius:9px; background:var(--card-bg) }.root-session-card p { margin:5px 0 9px; color:var(--text-secondary); font-size:9px }.root-session-row { display:flex; align-items:center; gap:9px; min-height:32px; padding:5px 7px; border-top:1px solid var(--border-subtle); color:var(--text-secondary); font-size:9px }.owner-badge,.auth-pill { padding:2px 6px; border-radius:999px; font-size:9px }.owner-badge.agent,.auth-pill { color:var(--accent-purple); background:var(--accent-purple-bg) }.owner-badge.user { color:var(--accent-primary); background:var(--primary-bg) }.owner-badge.other { color:var(--accent-warning); background:var(--warning-bg) }.owner-badge.unknown { color:var(--text-secondary); background:var(--surface-2) }
.recent-login-row { display:flex; align-items:center; gap:8px; margin-top:8px; padding:7px 8px; border:1px solid var(--border-default); border-radius:6px; color:var(--text-secondary); background:var(--surface-1); font-size:9px }.recent-login-row strong { color:var(--text-primary) }.recent-dot { width:6px; height:6px; border-radius:50%; background:var(--accent-warning) }
.collapse-head { width:100%; min-height:54px; display:flex; align-items:center; gap:10px; padding:9px 12px; border:0; background:var(--card-header-bg); color:var(--text-primary); text-align:left; cursor:pointer }.state-dot { width:8px; height:8px; border-radius:50%; background:var(--text-muted); box-shadow:0 0 0 3px var(--surface-2) }.state-root .state-dot,.state-migrating .state-dot { background:var(--accent-danger) }.state-ready .state-dot { background:var(--accent-primary) }.state-active .state-dot { background:var(--accent-success); box-shadow:0 0 0 3px var(--success-bg) }.agent-heading { display:flex; flex:1; min-width:0; flex-direction:column; gap:2px }.agent-heading strong { font-size:12px }.agent-heading small { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:var(--text-secondary); font:10px ui-monospace,SFMono-Regular,Menlo,monospace }.state-badge { padding:3px 7px; border-radius:999px; background:var(--surface-2); color:var(--text-secondary); font-size:9px }.state-root .state-badge,.state-migrating .state-badge { color:var(--accent-danger); background:var(--danger-bg) }.state-active .state-badge { color:var(--accent-success); background:var(--success-bg) }.detail-label { color:var(--text-muted); font-size:9px }.chevron { width:14px; transition:transform .15s }.chevron.open { transform:rotate(180deg) }.agent-body { padding:13px }
.lifecycle { display:grid; grid-template-columns:repeat(4,1fr); margin-bottom:14px }.lifecycle div { position:relative; display:flex; align-items:center; gap:6px; color:var(--text-muted); font-size:9px }.lifecycle div:not(:last-child)::after { content:''; height:1px; flex:1; background:var(--border-default); margin-right:6px }.lifecycle i { width:18px; height:18px; display:grid; place-items:center; border-radius:50%; border:1px solid var(--border-default); font-style:normal }.lifecycle .done { color:var(--accent-success) }.lifecycle .done i { border-color:var(--border-success); background:var(--success-bg) }.lifecycle .danger { color:var(--accent-danger) }.lifecycle .danger i { border-color:var(--border-danger); background:var(--danger-bg) }
.agent-meta { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:8px; margin:0 0 11px }.agent-meta div { padding:8px; background:var(--surface-1); border-radius:6px }.agent-meta dt { color:var(--text-muted); font-size:9px }.agent-meta dd { margin:4px 0 0; font-size:10px; overflow-wrap:anywhere }.key-list,.session-list { display:flex; flex-direction:column; gap:5px; margin-top:8px }.key-row,.session-row { min-height:34px; display:flex; align-items:center; justify-content:space-between; gap:8px; padding:6px 8px; border:1px solid var(--border-subtle); border-radius:6px; background:var(--surface-1) }.key-main { min-width:0; gap:8px }.key-main code { color:var(--text-primary); font-size:10px }.key-main small { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:var(--text-secondary); font-size:9px }.user-pill { padding:2px 6px; border-radius:999px; color:var(--accent-primary); background:var(--primary-bg); font-size:9px }.user-pill.root { color:var(--accent-danger); background:var(--danger-bg) }.danger-button { border:1px solid var(--border-danger); background:var(--danger-bg); color:var(--accent-danger); border-radius:5px; padding:4px 7px; font-size:9px; cursor:pointer }.waiting-box { color:var(--text-secondary); background:var(--surface-1); border:1px dashed var(--border-default) }.waiting-box code { color:var(--accent-purple) }.session-row { justify-content:flex-start; color:var(--text-secondary); font-size:9px }.session-row strong { color:var(--text-primary) }.live-dot { width:6px; height:6px; border-radius:50%; background:var(--accent-success) }.card-actions { justify-content:flex-end; gap:10px; margin-top:11px; padding-top:9px; border-top:1px solid var(--border-subtle); font-size:9px }.card-actions button:last-child { color:var(--text-muted) }.add-agent-button { margin-top:10px; width:100% }
.discovered-card { margin-top:14px; padding:12px; border:1px solid var(--border-warning); border-radius:9px; background:var(--warning-bg) }.section-heading { justify-content:space-between }.section-heading strong { font-size:11px }.section-heading span { color:var(--accent-warning) }.discovered-card p { color:var(--text-secondary); font-size:9px }.discovered-card .key-row { margin-top:5px }
.modal-mask { position:fixed; inset:0; z-index:10000; display:grid; place-items:center; background:var(--bg-overlay) }.register-modal { width:min(440px,calc(100vw - 32px)); padding:16px; border:1px solid var(--border-default); border-radius:11px; background:var(--bg-panel-solid); box-shadow:var(--shadow-lg) }.modal-head { display:flex; justify-content:space-between; margin-bottom:13px }.modal-head h3 { margin:0; font-size:15px }.modal-head p { margin:4px 0 0; color:var(--text-secondary); font-size:10px }.modal-head>button { border:0; background:none; color:var(--text-secondary); font-size:20px; cursor:pointer }.register-modal label { display:flex; flex-direction:column; gap:5px; margin:10px 0; color:var(--text-secondary); font-size:10px }.register-modal input { padding:8px 9px; border:1px solid var(--border-default); border-radius:6px; background:var(--bg-input); color:var(--text-primary); outline:none }.register-modal input:focus { border-color:var(--border-accent) }.register-note { color:var(--accent-warning); background:var(--warning-bg); border:1px solid var(--border-warning) }.modal-actions { justify-content:flex-end; gap:8px; margin-top:14px }.modal-actions>button:not(.primary-button) { border:1px solid var(--border-default); border-radius:6px; padding:7px 11px; background:var(--surface-1); color:var(--text-primary); cursor:pointer }
.cleanup-modal { width:min(540px,calc(100vw - 32px)) }.cleanup-warning { padding:9px 10px; border:1px solid var(--border-warning); border-radius:6px; color:var(--accent-warning); background:var(--warning-bg); font-size:10px; line-height:1.5 }.command-preview { margin:10px 0 0; padding:11px; max-height:230px; overflow:auto; white-space:pre-wrap; user-select:text; border:1px solid var(--border-default); border-radius:7px; background:var(--bg-terminal); color:var(--text-primary); font:10px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace }
@keyframes spin { to { transform:rotate(360deg) } }
@media (max-width:720px) { .external-agent-panel { min-width:0 }.agent-meta { grid-template-columns:1fr }.lifecycle { grid-template-columns:1fr 1fr; gap:6px }.lifecycle div::after { display:none }.detail-label { display:none } }
</style>
