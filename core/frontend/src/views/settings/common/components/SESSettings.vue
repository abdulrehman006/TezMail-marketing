<template>
	<div>
		<div class="flex items-center justify-between mb-12px">
			<span class="font-bold text-basic text-14px">{{ $t('settings.common.ses.title') }}</span>
			<n-button type="primary" size="small" @click="handleAdd">
				{{ $t('settings.common.ses.addAccount') }}
			</n-button>
		</div>

		<p class="text-desc text-12px mb-16px">{{ $t('settings.common.ses.description') }}</p>

		<!-- Account List -->
		<div v-if="sesAccounts.length > 0" class="space-y-16px">
			<n-card v-for="account in sesAccounts" :key="account.id" size="small">
				<div class="flex items-start justify-between">
					<div class="flex-1">
						<div class="flex items-center gap-8px mb-8px">
							<span class="font-medium">{{ account.name }}</span>
							<n-tag :type="getStatusType(account.status)" size="small">
								{{ account.status }}
							</n-tag>
							<n-switch
								:value="account.enabled"
								size="small"
								@update:value="(val: boolean) => handleToggleEnabled(account, val)" />
						</div>
						<p class="text-desc text-12px mb-4px">{{ account.description || '-' }}</p>
						<div class="grid grid-cols-2 gap-8px text-12px">
							<div>
								<span class="text-desc">{{ $t('settings.common.ses.region') }}:</span>
								<span class="ml-4px">{{ account.region }}</span>
							</div>
							<div>
								<span class="text-desc">{{ $t('settings.common.ses.accessKey') }}:</span>
								<span class="ml-4px">{{ account.access_key }}</span>
							</div>
							<div v-if="account.verified_domains?.length">
								<span class="text-desc">{{ $t('settings.common.ses.verifiedDomains') }}:</span>
								<span class="ml-4px">{{ account.verified_domains.join(', ') }}</span>
							</div>
							<div v-if="account.send_quota">
								<span class="text-desc">{{ $t('settings.common.ses.quota') }}:</span>
								<span class="ml-4px">
									{{ account.send_quota.sent_last_24_hours }} / {{ account.send_quota.max_24_hour_send }}
								</span>
							</div>
						</div>
						<div v-if="account.domains?.length" class="mt-8px">
							<span class="text-desc text-12px">{{ $t('settings.common.ses.mappedDomains') }}:</span>
							<n-tag v-for="domain in account.domains" :key="domain" size="small" class="ml-4px">
								{{ domain }}
							</n-tag>
						</div>
						<p v-if="account.status_message" class="text-desc text-12px mt-4px">
							{{ account.status_message }}
						</p>
					</div>
					<div class="flex gap-8px">
						<n-button text type="primary" size="small" @click="handleEdit(account)">
							{{ $t('common.actions.edit') }}
						</n-button>
						<n-button text type="error" size="small" @click="handleDelete(account)">
							{{ $t('common.actions.delete') }}
						</n-button>
					</div>
				</div>
			</n-card>
		</div>

		<n-empty v-else :description="$t('settings.common.ses.noAccounts')" />

		<!-- Add/Edit Modal -->
		<n-modal v-model:show="showModal" preset="card" :title="editingAccount ? $t('settings.common.ses.editAccount') : $t('settings.common.ses.addAccount')" style="width: 600px">
			<n-form ref="formRef" :model="formData" :rules="formRules" label-placement="left" label-width="120px">
				<n-form-item :label="$t('settings.common.ses.name')" path="name">
					<n-input v-model:value="formData.name" :placeholder="$t('settings.common.ses.namePlaceholder')" />
				</n-form-item>
				<n-form-item :label="$t('settings.common.ses.description')" path="description">
					<n-input v-model:value="formData.description" :placeholder="$t('settings.common.ses.descriptionPlaceholder')" />
				</n-form-item>
				<n-form-item :label="$t('settings.common.ses.region')" path="region">
					<n-select v-model:value="formData.region" :options="regionOptions" :placeholder="$t('settings.common.ses.regionPlaceholder')" />
				</n-form-item>
				<n-form-item :label="$t('settings.common.ses.accessKey')" path="access_key">
					<n-input v-model:value="formData.access_key" :placeholder="$t('settings.common.ses.accessKeyPlaceholder')" />
				</n-form-item>
				<n-form-item :label="$t('settings.common.ses.secretKey')" path="secret_key">
					<n-input
						v-model:value="formData.secret_key"
						type="password"
						show-password-on="click"
						:placeholder="editingAccount ? $t('settings.common.ses.secretKeyPlaceholderEdit') : $t('settings.common.ses.secretKeyPlaceholder')" />
				</n-form-item>
				<n-form-item :label="$t('settings.common.ses.domains')" path="domains">
					<n-dynamic-tags v-model:value="formData.domains" />
					<p class="text-desc text-12px mt-4px">{{ $t('settings.common.ses.domainsHelp') }}</p>
				</n-form-item>
				<n-form-item :label="$t('settings.common.ses.enabled')" path="enabled">
					<n-switch v-model:value="formData.enabled" />
				</n-form-item>
			</n-form>
			<template #footer>
				<div class="flex justify-end gap-8px">
					<n-button @click="handleTestConnection" :loading="testing">
						{{ $t('settings.common.ses.testConnection') }}
					</n-button>
					<n-button @click="showModal = false">{{ $t('common.actions.cancel') }}</n-button>
					<n-button type="primary" @click="handleSave" :loading="saving">
						{{ $t('common.actions.save') }}
					</n-button>
				</div>
			</template>
		</n-modal>
	</div>
</template>

<script lang="tsx" setup>
import { confirm } from '@/utils'
import { getSESConfig, saveSESAccount, deleteSESAccount, testSESConnection, type SESAccount } from '@/api/modules/settings/common'
import type { FormInst, FormRules } from 'naive-ui'

const { t } = useI18n()
const message = useMessage()

const sesAccounts = ref<SESAccount[]>([])
const showModal = ref(false)
const editingAccount = ref<SESAccount | null>(null)
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const testing = ref(false)

const formData = reactive({
	id: 0,
	name: '',
	description: '',
	region: '',
	access_key: '',
	secret_key: '',
	enabled: true,
	domains: [] as string[],
})

const regionOptions = [
	{ label: 'US East (N. Virginia) - us-east-1', value: 'us-east-1' },
	{ label: 'US East (Ohio) - us-east-2', value: 'us-east-2' },
	{ label: 'US West (N. California) - us-west-1', value: 'us-west-1' },
	{ label: 'US West (Oregon) - us-west-2', value: 'us-west-2' },
	{ label: 'EU (Ireland) - eu-west-1', value: 'eu-west-1' },
	{ label: 'EU (London) - eu-west-2', value: 'eu-west-2' },
	{ label: 'EU (Frankfurt) - eu-central-1', value: 'eu-central-1' },
	{ label: 'Asia Pacific (Mumbai) - ap-south-1', value: 'ap-south-1' },
	{ label: 'Asia Pacific (Singapore) - ap-southeast-1', value: 'ap-southeast-1' },
	{ label: 'Asia Pacific (Sydney) - ap-southeast-2', value: 'ap-southeast-2' },
	{ label: 'Asia Pacific (Tokyo) - ap-northeast-1', value: 'ap-northeast-1' },
	{ label: 'Canada (Central) - ca-central-1', value: 'ca-central-1' },
	{ label: 'South America (Sao Paulo) - sa-east-1', value: 'sa-east-1' },
]

const formRules: FormRules = {
	name: { required: true, message: t('settings.common.ses.nameRequired'), trigger: 'blur' },
	region: { required: true, message: t('settings.common.ses.regionRequired'), trigger: 'blur' },
	access_key: { required: true, message: t('settings.common.ses.accessKeyRequired'), trigger: 'blur' },
}

const getStatusType = (status: string) => {
	switch (status) {
		case 'connected':
			return 'success'
		case 'pending':
		case 'checking':
			return 'warning'
		case 'failed':
		case 'disabled':
			return 'error'
		default:
			return 'default'
	}
}

const loadSESConfig = async () => {
	try {
		const res = await getSESConfig()
		if (res && res.accounts) {
			sesAccounts.value = res.accounts
		}
	} catch (e) {
		console.error('Failed to load SES config:', e)
	}
}

const resetForm = () => {
	formData.id = 0
	formData.name = ''
	formData.description = ''
	formData.region = ''
	formData.access_key = ''
	formData.secret_key = ''
	formData.enabled = true
	formData.domains = []
}

const handleAdd = () => {
	resetForm()
	editingAccount.value = null
	showModal.value = true
}

const handleEdit = (account: SESAccount) => {
	editingAccount.value = account
	formData.id = account.id
	formData.name = account.name
	formData.description = account.description || ''
	formData.region = account.region
	formData.access_key = account.access_key
	formData.secret_key = ''
	formData.enabled = account.enabled
	formData.domains = account.domains || []
	showModal.value = true
}

const handleDelete = (account: SESAccount) => {
	confirm({
		title: t('settings.common.ses.deleteConfirm'),
		content: t('settings.common.ses.deleteConfirmContent', { name: account.name }),
		onConfirm: async () => {
			await deleteSESAccount({ id: account.id })
			loadSESConfig()
		},
	})
}

const handleToggleEnabled = async (account: SESAccount, enabled: boolean) => {
	try {
		await saveSESAccount({
			id: account.id,
			name: account.name,
			region: account.region,
			access_key: account.access_key,
			enabled: enabled,
			domains: account.domains,
		})
		loadSESConfig()
	} catch (e) {
		console.error('Failed to toggle enabled:', e)
	}
}

const handleTestConnection = async () => {
	if (!formData.region || !formData.access_key) {
		message.warning(t('settings.common.ses.fillCredentialsFirst'))
		return
	}
	// Secret key is always required for testing connection
	if (!formData.secret_key) {
		message.warning(t('settings.common.ses.secretKeyRequired'))
		return
	}

	testing.value = true
	try {
		const res = await testSESConnection({
			region: formData.region,
			access_key: formData.access_key,
			secret_key: formData.secret_key,
		})
		if (res && res.status === 'connected') {
			message.success(t('settings.common.ses.connectionSuccess'))
		}
	} catch (e) {
		console.error('Connection test failed:', e)
	} finally {
		testing.value = false
	}
}

const handleSave = async () => {
	try {
		await formRef.value?.validate()
	} catch {
		return
	}

	if (!editingAccount.value && !formData.secret_key) {
		message.warning(t('settings.common.ses.secretKeyRequired'))
		return
	}

	saving.value = true
	try {
		await saveSESAccount({
			id: formData.id || undefined,
			name: formData.name,
			description: formData.description,
			region: formData.region,
			access_key: formData.access_key,
			secret_key: formData.secret_key || undefined,
			enabled: formData.enabled,
			domains: formData.domains,
		})
		showModal.value = false
		loadSESConfig()
	} catch (e) {
		console.error('Failed to save:', e)
	} finally {
		saving.value = false
	}
}

onMounted(() => {
	loadSESConfig()
})
</script>

<style lang="scss" scoped></style>
