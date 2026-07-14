import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

const STORAGE_KEY = 'qssh.external-agents.v1'

function readStoredRecords() {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}')
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

export const useExternalAgentStore = defineStore('externalAgents', () => {
  // 只保存监管元数据。私钥、公钥正文、密码和服务器地址不会写入这里。
  const recordsByConnection = ref(readStoredRecords())

  const persist = () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(recordsByConnection.value))
  }

  const getRecords = (connId) => recordsByConnection.value[connId] || []

  const addRecord = (connId, record) => {
    if (!recordsByConnection.value[connId]) recordsByConnection.value[connId] = []
    recordsByConnection.value[connId].push({
      id: `external-agent-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
      name: record.name.trim(),
      keyLabel: record.keyLabel.trim(),
      serviceUser: record.serviceUser.trim(),
      fingerprint: record.fingerprint || '',
      workstationKeyPath: record.workstationKeyPath?.trim() || '',
      createdAt: new Date().toISOString()
    })
    persist()
  }

	const removeRecord = (connId, recordId) => {
    const records = recordsByConnection.value[connId] || []
    recordsByConnection.value[connId] = records.filter(record => record.id !== recordId)
    if (recordsByConnection.value[connId].length === 0) delete recordsByConnection.value[connId]
    persist()
	}

	const migrateRecords = (fromScope, toScope) => {
		if (!fromScope || fromScope === toScope || recordsByConnection.value[toScope] || !recordsByConnection.value[fromScope]) return false
		recordsByConnection.value[toScope] = recordsByConnection.value[fromScope]
		delete recordsByConnection.value[fromScope]
		persist()
		return true
	}

  const recordCount = computed(() => Object.values(recordsByConnection.value).reduce((sum, records) => sum + records.length, 0))

	return { recordsByConnection, recordCount, getRecords, addRecord, removeRecord, migrateRecords }
})
