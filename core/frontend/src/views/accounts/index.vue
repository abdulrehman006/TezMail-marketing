<template>
	<div class="p-24px">
		<div class="bt-title">{{ t('layout.menu.accounts') }}</div>
		<bt-table-layout>
			<template #toolsLeft>
				<n-button type="primary" @click="handleAdd">{{ t('rbac.account.actions.add') }}</n-button>
			</template>
			<template #toolsRight>
				<bt-search
					v-model:value="tableParams.username"
					:width="280"
					:placeholder="t('rbac.account.search.placeholder')"
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
				<account-password ref="passwordRef"></account-password>
			</template>
		</bt-table-layout>
	</div>
</template>

<script lang="tsx" setup>
import { DataTableColumns, NButton, NFlex, NTag } from 'naive-ui'
import { confirm } from '@/utils'
import { useModal } from '@/hooks/modal/useModal'
import { useDataTable } from '@/hooks/useDataTable'
import { getAccountList, deleteAccount } from '@/api/modules/rbac'
import { Account, AccountParams } from './interface'

import AccountForm from './components/AccountForm.vue'
import AccountPassword from './components/AccountPassword.vue'

const { t } = useI18n()

const passwordRef = useTemplateRef('passwordRef')

const { tableParams, tableProps, pageProps, fetchTable, resetTable } = useDataTable<
	Account,
	AccountParams
>({
	loading: true,
	immediate: true,
	params: {
		page: 1,
		page_size: 10,
		username: '',
	},
	rowKey: row => row.id,
	fetchFn: getAccountList,
	useParams: params => ({
		page: params.page,
		pageSize: params.page_size,
		username: params.username,
	}),
})

const formatTime = (sec: number) => (sec ? new Date(sec * 1000).toLocaleString() : '--')

const columns = ref<DataTableColumns<Account>>([
	{
		key: 'username',
		title: t('rbac.account.columns.username'),
		minWidth: 140,
		ellipsis: { tooltip: true },
	},
	{
		key: 'email',
		title: t('rbac.account.columns.email'),
		minWidth: 180,
		ellipsis: { tooltip: true },
		render: row => row.email || '--',
	},
	{
		key: 'status',
		title: t('rbac.account.columns.status'),
		width: 120,
		render: row => (
			<NTag type={row.status === 1 ? 'success' : 'warning'} size="small" bordered={false}>
				{row.status === 1 ? t('rbac.status.enabled') : t('rbac.status.disabled')}
			</NTag>
		),
	},
	{
		key: 'create_time',
		title: t('rbac.account.columns.createTime'),
		minWidth: 170,
		render: row => formatTime(row.create_time),
	},
	{
		title: t('common.columns.actions'),
		key: 'actions',
		align: 'right',
		width: 220,
		render: row => (
			<NFlex inline={true}>
				<NButton type="primary" text={true} onClick={() => handleEdit(row)}>
					{t('common.actions.edit')}
				</NButton>
				<NButton type="primary" text={true} onClick={() => handlePassword(row)}>
					{t('rbac.account.actions.resetPassword')}
				</NButton>
				<NButton type="error" text={true} onClick={() => handleDelete(row)}>
					{t('common.actions.delete')}
				</NButton>
			</NFlex>
		),
	},
])

const [FormModal, formModalApi] = useModal({
	component: AccountForm,
	state: { isEdit: false, refresh: fetchTable },
})

const handleAdd = () => {
	formModalApi.setState({ isEdit: false, row: null })
	formModalApi.open()
}

const handleEdit = (row: Account) => {
	formModalApi.setState({ isEdit: true, row })
	formModalApi.open()
}

const handlePassword = (row: Account) => {
	passwordRef.value?.open(row)
}

const handleDelete = (row: Account) => {
	confirm({
		title: t('rbac.account.delete.title'),
		content: t('rbac.account.delete.confirm', { name: row.username }),
		confirmText: t('common.actions.delete'),
		confirmType: 'error',
		onConfirm: async () => {
			await deleteAccount(row.id)
			fetchTable()
		},
	})
}
</script>
