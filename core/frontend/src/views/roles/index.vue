<template>
	<div class="p-24px">
		<div class="bt-title">{{ t('layout.menu.roles') }}</div>
		<bt-table-layout>
			<template #toolsLeft>
				<n-button type="primary" @click="handleAdd">{{ t('rbac.role.actions.add') }}</n-button>
			</template>
			<template #toolsRight>
				<bt-search
					v-model:value="tableParams.name"
					:width="280"
					:placeholder="t('rbac.role.search.placeholder')"
					@search="() => resetTable()">
				</bt-search>
			</template>
			<template #table>
				<n-data-table v-bind="tableProps" :columns="columns">
					<template #empty>
						<bt-table-help> </bt-table-help>
					</template>
				</n-data-table>
			</template>
			<template #pageRight>
				<bt-table-page v-bind="pageProps" @refresh="fetchTable"> </bt-table-page>
			</template>
			<template #modal>
				<form-modal></form-modal>
			</template>
		</bt-table-layout>
	</div>
</template>

<script lang="tsx" setup>
import { DataTableColumns, NButton, NFlex, NTag } from 'naive-ui'
import { confirm } from '@/utils'
import { useModal } from '@/hooks/modal/useModal'
import { useDataTable } from '@/hooks/useDataTable'
import { getRoleList, deleteRole } from '@/api/modules/rbac'
import { Role, RoleParams } from './interface'

import RoleForm from './components/RoleForm.vue'

const { t } = useI18n()

const { tableParams, tableProps, pageProps, fetchTable, resetTable } = useDataTable<Role, RoleParams>({
	loading: true,
	immediate: true,
	params: {
		page: 1,
		page_size: 10,
		name: '',
	},
	rowKey: row => row.id,
	fetchFn: getRoleList,
	useParams: params => ({
		page: params.page,
		pageSize: params.page_size,
		name: params.name,
	}),
})

const formatTime = (sec: number) => (sec ? new Date(sec * 1000).toLocaleString() : '--')

// The built-in admin role is protected from editing/deletion.
const isProtected = (row: Role) => row.name === 'admin'

const columns = ref<DataTableColumns<Role>>([
	{
		key: 'name',
		title: t('rbac.role.columns.name'),
		minWidth: 140,
		ellipsis: { tooltip: true },
	},
	{
		key: 'description',
		title: t('rbac.role.columns.description'),
		minWidth: 220,
		ellipsis: { tooltip: true },
		render: row => row.description || '--',
	},
	{
		key: 'status',
		title: t('rbac.role.columns.status'),
		width: 120,
		render: row => (
			<NTag type={row.status === 1 ? 'success' : 'warning'} size="small" bordered={false}>
				{row.status === 1 ? t('rbac.status.enabled') : t('rbac.status.disabled')}
			</NTag>
		),
	},
	{
		key: 'create_time',
		title: t('rbac.role.columns.createTime'),
		minWidth: 170,
		render: row => formatTime(row.create_time),
	},
	{
		title: t('common.columns.actions'),
		key: 'actions',
		align: 'right',
		width: 160,
		render: row => (
			<NFlex inline={true}>
				<NButton
					type="primary"
					text={true}
					disabled={isProtected(row)}
					onClick={() => handleEdit(row)}>
					{t('common.actions.edit')}
				</NButton>
				<NButton
					type="error"
					text={true}
					disabled={isProtected(row)}
					onClick={() => handleDelete(row)}>
					{t('common.actions.delete')}
				</NButton>
			</NFlex>
		),
	},
])

const [FormModal, formModalApi] = useModal({
	component: RoleForm,
	state: { isEdit: false, refresh: fetchTable },
})

const handleAdd = () => {
	formModalApi.setState({ isEdit: false, row: null })
	formModalApi.open()
}

const handleEdit = (row: Role) => {
	formModalApi.setState({ isEdit: true, row })
	formModalApi.open()
}

const handleDelete = (row: Role) => {
	confirm({
		title: t('rbac.role.delete.title'),
		content: t('rbac.role.delete.confirm', { name: row.name }),
		confirmText: t('common.actions.delete'),
		confirmType: 'error',
		onConfirm: async () => {
			await deleteRole(row.id)
			fetchTable()
		},
	})
}
</script>
