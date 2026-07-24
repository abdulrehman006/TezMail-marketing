<template>
	<modal :title="title" width="560">
		<bt-form ref="formRef" class="pt-20px" :model="form" :rules="rules">
			<n-form-item :label="t('rbac.role.form.name')" path="name">
				<n-input v-model:value="form.name" :placeholder="t('rbac.role.form.namePlaceholder')"> </n-input>
			</n-form-item>
			<n-form-item :label="t('rbac.role.form.description')" path="description">
				<n-input
					v-model:value="form.description"
					type="textarea"
					:autosize="{ minRows: 2, maxRows: 4 }"
					:placeholder="t('rbac.role.form.descriptionPlaceholder')">
				</n-input>
			</n-form-item>
			<n-form-item :label="t('rbac.role.form.permissions')" path="permissionIds">
				<n-checkbox-group v-model:value="form.permissionIds" class="w-full">
					<n-grid :cols="2" :x-gap="12" :y-gap="10">
						<n-gi v-for="p in permissionOptions" :key="p.id">
							<n-checkbox :value="p.id">
								<div class="flex flex-col">
									<span>{{ p.name }}</span>
									<span class="text-12px text-[var(--color-text-3)]">{{ p.description }}</span>
								</div>
							</n-checkbox>
						</n-gi>
					</n-grid>
				</n-checkbox-group>
			</n-form-item>
			<n-form-item :label="t('rbac.role.form.status')" :show-feedback="false">
				<n-switch v-model:value="form.status" :checked-value="1" :unchecked-value="0" />
			</n-form-item>
		</bt-form>
	</modal>
</template>

<script lang="ts" setup>
import { FormRules } from 'naive-ui'
import { useModal } from '@/hooks/modal/useModal'
import { createRole, updateRole, getPermissionList, getRoleDetail } from '@/api/modules/rbac'
import type { Permission, Role } from '../interface'

const { t } = useI18n()

const isEdit = ref(false)

const title = computed(() =>
	isEdit.value ? t('rbac.role.form.editTitle') : t('rbac.role.form.addTitle')
)

const formRef = useTemplateRef('formRef')

const permissionOptions = ref<Permission[]>([])

const form = reactive({
	roleId: 0,
	name: '',
	description: '',
	status: 1,
	permissionIds: [] as number[],
})

const rules: FormRules = {
	name: {
		required: true,
		trigger: 'blur',
		validator: () =>
			form.name.trim() === '' ? new Error(t('rbac.role.validation.nameRequired')) : true,
	},
}

const loadPermissions = async () => {
	const res = await getPermissionList()
	permissionOptions.value = ((res as any)?.list || []) as Permission[]
}

const reset = () => {
	form.roleId = 0
	form.name = ''
	form.description = ''
	form.status = 1
	form.permissionIds = []
}

const [Modal, modalApi] = useModal({
	onChangeState: async isOpen => {
		if (isOpen) {
			const state = modalApi.getState<{ isEdit: boolean; row: Role | null }>()
			isEdit.value = state.isEdit
			await loadPermissions()
			const { row } = state
			if (row) {
				form.roleId = row.id
				form.name = row.name
				form.description = row.description
				form.status = row.status
				form.permissionIds = []
				const detail = await getRoleDetail(row.id)
				const perms = (detail as any)?.permissions || []
				form.permissionIds = perms.map((p: any) => p.id)
			}
		} else {
			reset()
		}
	},
	onConfirm: async () => {
		await formRef.value?.validate()
		if (isEdit.value) {
			await updateRole({
				roleId: form.roleId,
				name: form.name,
				description: form.description,
				permissionIds: form.permissionIds,
				status: form.status,
			})
		} else {
			await createRole({
				name: form.name,
				description: form.description,
				permissionIds: form.permissionIds,
				status: form.status,
			})
		}
		const state = modalApi.getState<{ refresh: Function }>()
		state.refresh()
	},
})
</script>
