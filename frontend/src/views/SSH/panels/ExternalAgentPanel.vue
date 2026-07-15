<template>
  <div class="key-panel">
    <Message ref="messageRef" />

    <section class="key-manager-card">
      <header class="card-header">
        <div>
          <h2>密钥管理</h2>
          <p>按公钥指纹归类账号。撤销后可以恢复，彻底撤销后不可恢复。</p>
        </div>
        <button class="icon-button" :disabled="scanning" title="重新扫描" @click="scanAudit()">
          <svg :class="{ spinning: scanning }" viewBox="0 0 24 24"><path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7" /></svg>
        </button>
      </header>

      <div class="prompt-actions">
        <button class="primary-button" @click="copyTakeoverPrompt">复制 Agent 接管提示词</button>
        <button class="secondary-button" @click="copyUnloadPrompt">复制工作机私钥彻底删除提示词</button>
      </div>

      <div class="account-rule">
        <svg viewBox="0 0 24 24"><path d="M12 3 4.5 6v5.2c0 4.6 3.1 8 7.5 9.8 4.4-1.8 7.5-5.2 7.5-9.8V6L12 3Z" /><path d="M9 12.2 11 14l4-4" /></svg>
        <div>
          <strong>专属账号必须是普通账号</strong>
          <span>不加入 sudo、wheel、admin 等管理组，不写入 sudoers，不授予提权权限。是否保留同指纹的 root 授权由用户自行决定。</span>
        </div>
      </div>

      <div class="toolbar-row">
        <div class="summary">
          <strong>{{ fingerprintGroups.length }}</strong> 个指纹
          <span>·</span>
          <strong>{{ accountCount }}</strong> 个账号授权
          <template v-if="onlineSessionCount">
            <span>·</span>
            <strong class="online-summary">{{ onlineSessionCount }}</strong> 个指纹确认在线会话
          </template>
          <span v-if="audit && !audit.canInspectAll" class="scope-note">当前连接只能管理 {{ connectionUsername }}</span>
        </div>
      </div>

      <div v-if="scanning && !audit" class="empty-state">正在读取服务器密钥…</div>
      <div v-else-if="!fingerprintGroups.length" class="empty-state">
        <strong>还没有发现授权密钥</strong>
        <span>连接服务器后点击右上角刷新。扫描不会返回公钥正文或私钥。</span>
      </div>

      <div v-else class="fingerprint-list">
        <article v-for="group in fingerprintGroups" :key="group.fingerprint" class="fingerprint-group">
          <div class="fingerprint-head">
            <div class="fingerprint-title">
              <span class="key-mark"><svg viewBox="0 0 24 24"><circle cx="8" cy="10" r="4"/><path d="m11 13 8 8m-3-3 2-2m-5-1 2-2"/></svg></span>
              <div>
                <code>{{ group.fingerprint }}</code>
                <small>{{ group.comment || '无标签' }} · {{ group.algorithm }}</small>
              </div>
            </div>
            <span class="account-total">{{ group.accounts.length }} 个账号</span>
          </div>

          <div class="account-list">
            <div v-for="account in group.accounts" :key="account.username" class="account-row">
              <div class="account-identity">
                <span class="account-icon">{{ account.username.slice(0, 1).toUpperCase() }}</span>
                <div>
                  <strong>{{ account.username }}</strong>
                  <small :class="accountPrivilegeClass(group, account)">{{ accountPrivilegeLabel(group, account) }}</small>
                  <small v-if="account.sessions.length" class="online-detail">
                    {{ sessionDetail(account.sessions[0]) }}<template v-if="account.sessions.length > 1"> · 另有 {{ account.sessions.length - 1 }} 个会话</template>
                  </small>
                </div>
              </div>
              <div class="status-stack">
                <span v-if="account.sessions.length" class="status online">当前在线 {{ account.sessions.length }}</span>
                <span class="status" :class="account.revoked ? 'revoked' : 'active'">
                  {{ account.revoked ? '已撤销，可恢复' : '可登录' }}
                </span>
              </div>
              <div class="row-actions">
                <button class="permission-button" :disabled="!canManage(account)" :title="canManage(account) ? '查看账号权限' : '当前连接无权检查该账号'" @click="togglePermissions(group, account)">
                  {{ isPermissionExpanded(group, account) ? '收起权限' : '权限' }}
                </button>
                <button v-if="account.revoked" class="restore-button" :disabled="!canManage(account)" @click="restoreKey(group, account)">恢复</button>
                <button v-else class="revoke-button" :disabled="!canManage(account)" @click="revokeKey(group, account)">撤销</button>
                <button class="permanent-button" :disabled="!canManage(account)" @click="permanentlyRevokeKey(group, account)">彻底撤销</button>
              </div>
              <div v-if="isPermissionExpanded(group, account)" class="permission-details">
                <div v-if="isPermissionLoading(group, account)" class="permission-loading">正在只读检查账号权限…</div>
                <template v-else-if="permissionSnapshot(group, account)">
                  <div class="permission-summary-row">
                    <strong>账号权限</strong>
                    <span class="permission-mode" :class="`mode-${permissionSnapshot(group, account).sudoMode}`">
                      {{ sudoModeLabel(permissionSnapshot(group, account).sudoMode) }}
                    </span>
                  </div>
                  <div class="permission-groups">
                    <span>所属用户组</span>
                    <code>{{ permissionSnapshot(group, account).groups.join(' · ') || '未读取到' }}</code>
                  </div>
                  <div v-if="permissionSnapshot(group, account).sudoRules.length" class="permission-rules">
                    <span>允许执行的 sudo 规则</span>
                    <code v-for="rule in permissionSnapshot(group, account).sudoRules" :key="rule">{{ rule }}</code>
                  </div>
                  <p v-else-if="permissionSnapshot(group, account).sudoMode === 'none'">没有发现可执行的 sudo 规则。</p>
                  <p v-else>服务器没有返回足够信息，暂时无法确认 sudo 权限。</p>
                </template>
              </div>
            </div>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup>
import { computed, inject, onMounted, onUnmounted, ref } from 'vue'
import {
  GetConnection,
  GetExternalAgentAccountPermissions,
  GetExternalAgentAudit,
  PermanentlyRevokeExternalAgentKey,
  RestoreExternalAgentKey,
  RevokeExternalAgentKey
} from '../../../../bindings/changeme/ssh/sshservice.js'
import Message from '../../../components/Message.vue'
import { useConfirm } from '../../../utils/confirm'

const connId = inject('connId')
const { confirm } = useConfirm()
const messageRef = ref(null)
const audit = ref(null)
const scanning = ref(false)
const connectionUsername = ref('')
const expandedPermissions = ref({})
const permissionSnapshots = ref({})
const permissionLoading = ref({})

const fingerprintGroups = computed(() => {
  const groups = new Map()
  for (const key of audit.value?.keys || []) {
    if (!groups.has(key.fingerprint)) {
      groups.set(key.fingerprint, {
        fingerprint: key.fingerprint,
        algorithm: key.algorithm,
        comment: key.comment,
        accounts: []
      })
    }
    const group = groups.get(key.fingerprint)
    const existing = group.accounts.find(item => item.username === key.username)
    if (!existing || (existing.revoked && !key.revoked)) {
      if (existing) group.accounts.splice(group.accounts.indexOf(existing), 1)
      group.accounts.push(key)
    }
  }
  return [...groups.values()]
    .map(group => ({
      ...group,
      accounts: group.accounts
        .map(account => ({
          ...account,
          sessions: (audit.value?.sessions || []).filter(session =>
            session.active && session.fingerprint === group.fingerprint && session.username === account.username
          )
        }))
        .sort((a, b) => b.sessions.length - a.sessions.length || Number(a.revoked) - Number(b.revoked) || a.username.localeCompare(b.username))
    }))
    .sort((a, b) => a.comment.localeCompare(b.comment) || a.fingerprint.localeCompare(b.fingerprint))
})

const accountCount = computed(() => fingerprintGroups.value.reduce((sum, group) => sum + group.accounts.length, 0))
const onlineSessionCount = computed(() => fingerprintGroups.value.reduce(
  (sum, group) => sum + group.accounts.reduce((accountSum, account) => accountSum + account.sessions.length, 0),
  0
))

function canManage(account) {
  return connectionUsername.value === 'root' || connectionUsername.value === account.username
}

function maskSource(value) {
  if (!value) return ''
  if (value.includes(':')) return value.replace(/:[^:]+$/, ':…')
  const parts = value.split('.')
  return parts.length === 4 ? `${parts[0]}.${parts[1]}.*.*` : value
}

function sessionDetail(session) {
  const terminal = session.terminal && session.terminal !== 'recent-auth' ? session.terminal : 'SSH 会话'
  const source = maskSource(session.source)
  return source ? `指纹确认在线 · ${terminal} · 来源 ${source}` : `指纹确认在线 · ${terminal}`
}

function permissionKey(group, account) {
  return `${group.fingerprint}\u0000${account.username}`
}

function isPermissionExpanded(group, account) {
  return expandedPermissions.value[permissionKey(group, account)] === true
}

function isPermissionLoading(group, account) {
  return permissionLoading.value[permissionKey(group, account)] === true
}

function permissionSnapshot(group, account) {
  return permissionSnapshots.value[permissionKey(group, account)] || null
}

function sudoModeLabel(mode) {
  return { none: '无 sudo 权限', limited: '受限 sudo', full: '完整 sudo', unknown: '权限未确认' }[mode] || '权限未确认'
}

function accountPrivilegeLabel(group, account) {
  if (account.isRoot) return 'root 账号 · 完整系统权限'
  const permissions = permissionSnapshot(group, account)
  if (!permissions) return '非 root 账号 · 权限待检查'
  return {
    none: '低权限账号 · 无 sudo',
    limited: '受限运维账号 · 有精确 sudo',
    full: '管理员账号 · 完整 sudo',
    unknown: '非 root 账号 · 权限未确认'
  }[permissions.sudoMode] || '非 root 账号 · 权限未确认'
}

function accountPrivilegeClass(group, account) {
  if (account.isRoot) return 'privilege-danger'
  const mode = permissionSnapshot(group, account)?.sudoMode
  return mode === 'full' ? 'privilege-danger' : mode === 'limited' ? 'privilege-warning' : mode === 'none' ? 'privilege-safe' : ''
}

async function togglePermissions(group, account) {
  const key = permissionKey(group, account)
  if (expandedPermissions.value[key]) {
    expandedPermissions.value[key] = false
    return
  }
  expandedPermissions.value[key] = true
  if (permissionSnapshots.value[key] || permissionLoading.value[key]) return
  permissionLoading.value[key] = true
  try {
    permissionSnapshots.value[key] = await GetExternalAgentAccountPermissions(connId, account.username)
  } catch (error) {
    expandedPermissions.value[key] = false
    messageRef.value?.error('权限检查失败：' + (error?.message || error))
  } finally {
    permissionLoading.value[key] = false
  }
}

function buildTakeoverPrompt() {
  return `你将从当前工作机通过 SSH 接管一台由“舟SSH”管理的服务器。

这是一份提供给所有舟SSH用户的通用接管协议。不得假定任何预设名称、账号、服务器、指纹或密钥路径，也不得沿用其他用户或其他服务器的数据。

必须遵守以下规则：
1. 首先让当前用户选择一次命名方式：用户可以自行指定“显示名称、公钥管理标签、专属账号名”，也可以直接回复“随机”。如果用户表示无所谓、未提供名称或选择随机，就为本次接管生成一组全新的 8 位小写字母数字后缀；显示名称使用 Agent-<后缀>，公钥标签和专属账号使用 qssh-agent-<后缀>。不得复用示例值或其他用户的数据。
2. 校验用户自定义或随机生成的公钥标签只能包含字母、数字、点、下划线和连字符；专属账号必须符合 Linux 用户名格式 ^[a-z_][a-z0-9_-]{0,31}$。如果服务器上已经存在同名账号且并非本次接管专用账号，要求用户换名或重新随机生成，不得接管已有陌生账号。
3. 使用最终确定的公钥标签生成一把独立 Ed25519 密钥。私钥只保存在当前工作机，不得输出、上传、粘贴进聊天或写入远端；不得复用个人默认密钥，不得启用 Agent Forwarding，不得使用 StrictHostKeyChecking=no。
4. 只向当前用户报告最终显示名称、专属账号、公钥标签、私钥路径、公钥路径、完整 SHA-256 指纹和完整公钥一行，绝不显示私钥正文。
5. 可以在当前用户明确授权后临时使用 root 完成首次部署，但不强制在接管结束时删除该指纹的 root 授权；是否保留或撤销 root，由当前用户在舟SSH密钥管理卡中自行决定。
6. 必须创建或确认本次确定的专属账号。该账号不得加入 sudo、wheel、admin 等管理组，不得写入 sudoers，不得拥有 sudo、su、doas、pkexec 等提权能力，不设置可用密码，只授予其主目录和任务所需普通文件的最小读写权限。
7. 将同一公钥写入专属账号的 authorized_keys，设置正确的所有者以及目录 700、文件 600 权限，并从一个全新 SSH 会话验证该账号能登录。不得以扩大系统权限来解决任务失败。
8. 首次出现服务器主机指纹时要求当前用户核对，不得绕过。发现目标服务器不符、需要删除用户数据、修改 SSH 全局配置、开放额外端口或需要任何提权时，立即停止并说明原因。
9. 完成后报告服务器、专属账号、公钥指纹、密钥标签、各账号当前授权、修改内容和遗留风险。提醒当前用户：以后可在舟SSH中按“指纹 → 账号”分别撤销、恢复或彻底撤销公钥。

现在先确定命名方式；名称确定后生成独立密钥，输出上述允许报告的信息以及已经替换好完整公钥的安装命令，然后停下来等待当前用户确认。`
}

function buildUnloadPrompt() {
  return `请帮助当前用户在保存 SSH 私钥的工作机上彻底删除舟SSH监管的私钥，不要在服务器上执行。

这是一份提供给所有舟SSH用户的通用彻底删除协议。不得假定私钥路径、公钥指纹、标签或账号，也不得使用其他用户、其他服务器或以前任务中的值。

说明：ssh-agent 不是文件夹，而是当前工作机上缓存已加载私钥的后台进程；SSH_AUTH_SOCK 指向它的临时通信 socket。私钥本身是磁盘文件，通常位于 ~/.ssh/，但必须以当前用户实际确认的路径为准。

1. 先让当前用户提供或确认本次要删除的完整 SHA-256 公钥指纹和工作机私钥路径。如果当前对话已经明确包含这两项，可以复述并让用户确认一次；信息不完整时必须询问，不得猜测或扫描后擅自选择。
2. 确认路径真实存在、不是目录且属于当前用户。执行 ssh-keygen -lf "<当前用户确认的私钥路径>.pub"，核对输出的完整 SHA-256 指纹与当前用户指定的目标指纹完全一致；不一致立即停止。
3. 从当前工作机的 ssh-agent 内存缓存中卸载这一把私钥：ssh-add -d "<当前用户确认的私钥路径>"。不要使用 ssh-add -D，不能影响其他密钥。如果当前没有运行 ssh-agent 或该密钥未加载，可以继续删除磁盘文件，但要如实记录。
4. 默认彻底删除磁盘上的私钥和公钥文件，不保留副本：rm -f -- "<当前用户确认的私钥路径>" "<当前用户确认的私钥路径>.pub"。不得删除父目录、其他同名前缀文件、known_hosts、authorized_keys 或任何其他密钥。
5. 删除后验证两处状态：用 test ! -e 分别确认私钥和 .pub 已不存在；如果 ssh-agent 可用，用 ssh-add -l 确认目标指纹已不在已加载列表中。任一验证失败都必须明确报告，不得宣称清理完成。
6. 最终只报告已核对的指纹、ssh-agent 内存卸载结果和磁盘文件删除结果；绝不显示私钥正文。

说明：这里的“内存删除”特指从 ssh-agent 的已加载密钥列表卸载。普通文件删除无法承诺对 SSD、快照或备份进行物理覆写；不要虚假声称已经完成不可恢复的物理擦除。`
}

async function copyText(text, success) {
  try {
    await navigator.clipboard.writeText(text)
    messageRef.value?.success(success)
  } catch {
    messageRef.value?.error('复制失败，请检查剪贴板权限')
  }
}

function copyTakeoverPrompt() { return copyText(buildTakeoverPrompt(), 'Agent 接管提示词已复制') }
function copyUnloadPrompt() { return copyText(buildUnloadPrompt(), '工作机私钥彻底删除提示词已复制') }

async function scanAudit(silent = false) {
  if (scanning.value) return
  scanning.value = true
  try {
    audit.value = await GetExternalAgentAudit(connId)
    if (!silent) messageRef.value?.success(`已读取 ${fingerprintGroups.value.length} 个指纹、${accountCount.value} 个账号授权`)
  } catch (error) {
    if (!silent) messageRef.value?.error('扫描失败：' + (error?.message || error))
  } finally {
    scanning.value = false
  }
}

async function revokeKey(group, account) {
  const ok = await confirm({
    title: `撤销 ${account.username} 的登录权限`,
    message: `将暂停指纹 ${group.fingerprint} 对 ${account.username} 的登录授权。公钥会保留在可恢复区，之后可以恢复。已建立的 SSH 会话不会自动断开。`,
    danger: false
  })
  if (!ok) return
  try {
    await RevokeExternalAgentKey(connId, account.username, group.fingerprint)
    await scanAudit(true)
    messageRef.value?.success(`已撤销 ${account.username} 的授权，可随时恢复`)
  } catch (error) {
    messageRef.value?.error('撤销失败：' + (error?.message || error))
  }
}

async function restoreKey(group, account) {
  try {
    await RestoreExternalAgentKey(connId, account.username, group.fingerprint)
    await scanAudit(true)
    messageRef.value?.success(`已恢复 ${account.username} 的公钥授权`)
  } catch (error) {
    messageRef.value?.error('恢复失败：' + (error?.message || error))
  }
}

async function permanentlyRevokeKey(group, account) {
  const ok = await confirm({
    title: `彻底撤销 ${account.username} 的公钥`,
    message: `将从 ${account.username} 的当前授权和可恢复区彻底删除指纹 ${group.fingerprint}。此操作无法在舟SSH中恢复，需要重新安装公钥才能再次登录。`,
    danger: true
  })
  if (!ok) return
  try {
    await PermanentlyRevokeExternalAgentKey(connId, account.username, group.fingerprint)
    await scanAudit(true)
    messageRef.value?.success(`已彻底撤销 ${account.username} 的公钥`)
  } catch (error) {
    messageRef.value?.error('彻底撤销失败：' + (error?.message || error))
  }
}

onMounted(async () => {
  try {
    const connection = await GetConnection(connId)
    connectionUsername.value = connection?.username || ''
  } catch {}
  await scanAudit(true)
  auditTimer = window.setInterval(() => {
    if (document.visibilityState === 'visible') scanAudit(true)
  }, 15000)
})

let auditTimer = null
onUnmounted(() => {
  if (auditTimer) window.clearInterval(auditTimer)
})
</script>

<style scoped>
.key-panel { height:100%; overflow:auto; padding:18px; background:var(--toolbar-4); color:var(--text-primary); min-width:480px }
.key-manager-card { max-width:920px; margin:0 auto; border:1px solid var(--border-default); border-radius:12px; background:var(--card-bg); box-shadow:var(--shadow-sm); overflow:hidden }
.card-header { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; padding:18px 18px 14px }
.card-header h2 { margin:0; font-size:17px; letter-spacing:-.01em }
.card-header p { margin:5px 0 0; color:var(--text-secondary); font-size:10px; line-height:1.5 }
button,input { font:inherit }
button { cursor:pointer }
button:disabled { cursor:not-allowed; opacity:.42 }
svg { width:18px; height:18px; fill:none; stroke:currentColor; stroke-width:1.8; stroke-linecap:round; stroke-linejoin:round }
.icon-button { width:30px; height:30px; display:grid; place-items:center; flex:0 0 auto; border:1px solid var(--border-default); border-radius:7px; color:var(--text-secondary); background:var(--surface-1) }
.prompt-actions { display:flex; gap:8px; padding:0 18px 16px }
.primary-button,.secondary-button { min-height:32px; padding:0 12px; border-radius:7px; font-size:10px; font-weight:600 }
.primary-button { border:1px solid var(--accent-primary); color:white; background:var(--accent-primary) }
.secondary-button { border:1px solid var(--border-default); color:var(--text-primary); background:var(--surface-1) }
.account-rule { display:flex; gap:10px; margin:0 18px 16px; padding:11px 12px; border:1px solid var(--border-success); border-radius:8px; color:var(--accent-success); background:var(--success-bg) }
.account-rule svg { flex:0 0 auto; margin-top:1px }
.account-rule div { display:flex; flex-direction:column; gap:3px }
.account-rule strong { font-size:10px }
.account-rule span { color:var(--text-secondary); font-size:9px; line-height:1.55 }
.toolbar-row { min-height:39px; display:flex; align-items:center; justify-content:space-between; gap:12px; padding:0 18px; border-top:1px solid var(--border-subtle); border-bottom:1px solid var(--border-subtle); background:var(--card-header-bg) }
.summary { color:var(--text-secondary); font-size:9px }
.summary strong { color:var(--text-primary) }
.summary .online-summary { color:var(--accent-success) }
.scope-note { margin-left:8px; color:var(--accent-warning) }
.fingerprint-list { padding:10px }
.fingerprint-group { border:1px solid var(--border-subtle); border-radius:8px; overflow:hidden }
.fingerprint-group + .fingerprint-group { margin-top:8px }
.fingerprint-head { min-height:50px; display:flex; align-items:center; justify-content:space-between; gap:12px; padding:8px 10px; background:var(--surface-1) }
.fingerprint-title { display:flex; align-items:center; min-width:0; gap:9px }
.key-mark { width:28px; height:28px; display:grid; place-items:center; flex:0 0 auto; border-radius:7px; color:var(--accent-purple); background:var(--accent-purple-bg) }
.key-mark svg { width:15px; height:15px }
.fingerprint-title div { min-width:0; display:flex; flex-direction:column; gap:3px }
.fingerprint-title code { overflow:hidden; text-overflow:ellipsis; color:var(--text-primary); font:10px ui-monospace,SFMono-Regular,Menlo,monospace; user-select:text }
.fingerprint-title small { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:var(--text-secondary); font-size:9px }
.account-total { flex:0 0 auto; color:var(--text-muted); font-size:9px }
.account-list { padding:0 10px }
.account-row { min-height:48px; display:grid; grid-template-columns:minmax(150px,1fr) auto minmax(215px,auto); align-items:center; gap:12px; border-top:1px solid var(--border-subtle) }
.account-identity { display:flex; align-items:center; min-width:0; gap:8px }
.account-icon { width:24px; height:24px; display:grid; place-items:center; flex:0 0 auto; border-radius:50%; color:var(--text-secondary); background:var(--surface-2); font-size:9px; font-weight:700 }
.account-identity div { min-width:0; display:flex; flex-direction:column; gap:2px }
.account-identity strong { font-size:10px }
.account-identity small { color:var(--text-muted); font-size:8px }
.account-identity .privilege-safe { color:var(--accent-success) }
.account-identity .privilege-warning { color:var(--accent-warning) }
.account-identity .privilege-danger { color:var(--accent-danger) }
.account-identity .online-detail { color:var(--accent-success); white-space:normal }
.status-stack { display:flex; align-items:flex-end; flex-direction:column; gap:3px }
.status { padding:3px 7px; border-radius:999px; font-size:8px; white-space:nowrap }
.status.active { color:var(--accent-success); background:var(--success-bg) }
.status.revoked { color:var(--accent-warning); background:var(--warning-bg) }
.status.online { color:var(--accent-success); border:1px solid var(--border-success); background:var(--success-bg) }
.row-actions { display:flex; justify-content:flex-end; gap:5px }
.row-actions button { padding:4px 7px; border-radius:5px; font-size:8px; background:transparent }
.permission-button { border:1px solid var(--border-default); color:var(--text-secondary) }
.revoke-button { border:1px solid var(--border-warning); color:var(--accent-warning) }
.restore-button { border:1px solid var(--border-success); color:var(--accent-success) }
.permanent-button { border:1px solid var(--border-danger); color:var(--accent-danger) }
.permission-details { grid-column:1/-1; width:100%; box-sizing:border-box; margin:-1px 0 8px; padding:10px; border:1px solid var(--border-subtle); border-radius:7px; background:var(--surface-1); font-size:9px }
.permission-loading { color:var(--text-secondary) }
.permission-summary-row { display:flex; align-items:center; justify-content:space-between; gap:10px }
.permission-summary-row strong { font-size:10px }
.permission-mode { padding:3px 7px; border-radius:999px; font-size:8px }
.mode-none { color:var(--accent-success); background:var(--success-bg) }
.mode-limited { color:var(--accent-warning); background:var(--warning-bg) }
.mode-full { color:var(--accent-danger); background:var(--danger-bg) }
.mode-unknown { color:var(--text-secondary); background:var(--surface-2) }
.permission-groups,.permission-rules { display:flex; flex-direction:column; gap:5px; margin-top:9px; color:var(--text-secondary) }
.permission-groups code,.permission-rules code { overflow-wrap:anywhere; padding:6px 7px; border:1px solid var(--border-subtle); border-radius:5px; color:var(--text-primary); background:var(--bg-terminal); font:9px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace; user-select:text }
.permission-details p { margin:8px 0 0; color:var(--text-secondary) }
.empty-state { min-height:170px; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:6px; padding:24px; text-align:center; color:var(--text-secondary); font-size:9px }
.empty-state strong { color:var(--text-primary); font-size:11px }
.spinning { animation:spin .8s linear infinite }
@keyframes spin { to { transform:rotate(360deg) } }
@media (max-width:720px) {
  .key-panel { min-width:0; padding:10px }
  .prompt-actions { flex-direction:column }
  .account-row { grid-template-columns:1fr auto; padding:8px 0 }
  .status { justify-self:end }
  .row-actions { grid-column:1/-1 }
}
</style>
