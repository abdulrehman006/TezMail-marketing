<template>
	<bt-modal
		v-model:show="show"
		:title="t('rbac.account.password.title', { name: form.username })"
		width="460"
		:on-confirm="onConfirm">
		<bt-form ref="formRef" class="pt-12px" :model="form" :rules="rules">
			<n-form-item :label="t('rbac.account.password.newPassword')" path="newPassword">
				<n-input
					v-model:value="form.newPassword"
					:placeholder="t('rbac.account.form.passwordPlaceholder')">
				</n-input>
			</n-form-item>
		</bt-form>
	</bt-modal>
</template>

<script lang="ts" setup>
import { FormRules } from 'naive-ui'
import { getRandomPassword } from '@/utils'
import { updateAccountPassword } from '@/api/modules/rbac'
import type { Account } from '../interface'

const { t } = useI18n()

const show = ref(false)

const formRef = useTemplateRef('formRef')

const form = reactive({
	accountId: 0,
	username: '',
	newPassword: '',
})

const rules: FormRules = {
	newPassword: {
		required: true,
		trigger: 'blur',
		validator: () =>
			form.newPassword.trim().length < 8
				? new Error(t('rbac.account.validation.passwordLength'))
				: true,
	},
}

const open = (row: Account) => {
	form.accountId = row.id
	form.username = row.username
	form.newPassword = getRandomPassword()
	show.value = true
}

const onConfirm = async () => {
	try {
		await formRef.value?.validate()
	} catch {
		return false
	}
	await updateAccountPassword({ accountId: form.accountId, newPassword: form.newPassword })
}

defineExpose({ open })
</script>
