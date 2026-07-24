<template>
	<modal :title="title" width="520">
		<bt-form ref="formRef" class="pt-20px" :model="form" :rules="rules">
			<n-form-item :label="t('rbac.account.form.username')" path="username">
				<n-input v-model:value="form.username" :placeholder="t('rbac.account.form.usernamePlaceholder')">
				</n-input>
			</n-form-item>
			<n-form-item :label="t('rbac.account.form.email')" path="email">
				<n-input v-model:value="form.email" :placeholder="t('rbac.account.form.emailPlaceholder')">
				</n-input>
			</n-form-item>
			<n-form-item v-if="!isEdit" :label="t('rbac.account.form.password')" path="password">
				<n-input
					v-model:value="form.password"
					:placeholder="t('rbac.account.form.passwordPlaceholder')">
				</n-input>
			</n-form-item>
			<n-form-item :label="t('rbac.account.form.roles')" path="roleIds">
				<n-select
					v-model:value="form.roleIds"
					multiple
					:options="roleOptions"
					:placeholder="t('rbac.account.form.rolesPlaceholder')">
				</n-select>
			</n-form-item>
			<n-form-item :label="t('rbac.account.form.status')" :show-feedback="false">
				<n-switch v-model:value="form.status" :checked-value="1" :unchecked-value="0" />
			</n-form-item>
		</bt-form>
	</modal>
</template>

<script lang="ts" setup>
import { FormRules } from 'naive-ui'
import { getRandomPassword } from '@/utils'
import { useModal } from '@/hooks/modal/useModal'
import {
	createAccount,
	updateAccount,
	getRoleList,
	getAccountDetail,
} from '@/api/modules/rbac'
import type { Account } from '../interface'

const { t } = useI18n()

const isEdit = ref(false)

const title = computed(() =>
	isEdit.value ? t('rbac.account.form.editTitle') : t('rbac.account.form.addTitle')
)

const formRef = useTemplateRef('formRef')

const roleOptions = ref<{ label: string; value: number }[]>([])

const form = reactive({
	accountId: 0,
	username: '',
	email: '',
	password: getRandomPassword(),
	status: 1,
	roleIds: [] as number[],
})

const rules: FormRules = {
	username: {
		required: true,
		trigger: 'blur',
		validator: () =>
			form.username.trim() === '' ? new Error(t('rbac.account.validation.usernameRequired')) : true,
	},
	email: {
		required: true,
		trigger: 'blur',
		validator: () => {
			if (form.email.trim() === '') {
				return new Error(t('rbac.account.validation.emailRequired'))
			}
			const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
			return re.test(form.email) ? true : new Error(t('rbac.account.validation.emailInvalid'))
		},
	},
	password: {
		trigger: 'blur',
		validator: () => {
			if (isEdit.value) return true
			if (form.password.trim().length < 8) {
				return new Error(t('rbac.account.validation.passwordLength'))
			}
			return true
		},
	},
}

const loadRoleOptions = async () => {
	const res = await getRoleList({ page: 1, pageSize: 1000 })
	const list = (res as any)?.list || []
	roleOptions.value = list.map((r: any) => ({ label: r.name, value: r.id }))
}

const reset = () => {
	form.accountId = 0
	form.username = ''
	form.email = ''
	form.password = getRandomPassword()
	form.status = 1
	form.roleIds = []
}

const [Modal, modalApi] = useModal({
	onChangeState: async isOpen => {
		if (isOpen) {
			const state = modalApi.getState<{ isEdit: boolean; row: Account | null }>()
			isEdit.value = state.isEdit
			await loadRoleOptions()
			const { row } = state
			if (row) {
				form.accountId = row.id
				form.username = row.username
				form.email = row.email
				form.status = row.status
				form.roleIds = []
				// Load the account's currently-assigned roles.
				const detail = await getAccountDetail(row.id)
				const roles = (detail as any)?.roles || []
				form.roleIds = roles.map((r: any) => r.id)
			}
		} else {
			reset()
		}
	},
	onConfirm: async () => {
		await formRef.value?.validate()
		if (isEdit.value) {
			await updateAccount({
				accountId: form.accountId,
				username: form.username,
				email: form.email,
				roleIds: form.roleIds,
				status: form.status,
			})
		} else {
			await createAccount({
				username: form.username,
				password: form.password,
				email: form.email,
				roleIds: form.roleIds,
				status: form.status,
			})
		}
		const state = modalApi.getState<{ refresh: Function }>()
		state.refresh()
	},
})
</script>
