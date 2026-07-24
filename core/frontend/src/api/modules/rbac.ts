import { instance } from '@/api'
import i18n from '@/i18n'

const { t } = i18n.global

// getCurrentUser lives in ./user alongside the rest of the auth lifecycle.

// ---- Accounts -------------------------------------------------------------

export const getAccountList = (params: {
	page: number
	pageSize: number
	username?: string
	email?: string
	status?: number
}) => instance.get('/account/list', { params })

export const getAccountDetail = (accountId: number) =>
	instance.get('/account/detail', { params: { accountId } })

export const createAccount = (params: {
	username: string
	password: string
	email: string
	roleIds: number[]
	status: number
	lang?: string
}) =>
	instance.post('/account/create', params, {
		fetchOptions: { loading: t('rbac.api.loading.saving'), successMessage: true },
	})

export const updateAccount = (params: {
	accountId: number
	username: string
	email: string
	roleIds: number[]
	status: number
}) =>
	instance.post('/account/update', params, {
		fetchOptions: { loading: t('rbac.api.loading.saving'), successMessage: true },
	})

export const updateAccountPassword = (params: {
	accountId: number
	newPassword: string
	oldPassword?: string
}) =>
	instance.post('/account/password', params, {
		fetchOptions: { loading: t('rbac.api.loading.saving'), successMessage: true },
	})

export const deleteAccount = (accountId: number) =>
	instance.post(
		'/account/delete',
		{ accountId },
		{ fetchOptions: { loading: t('rbac.api.loading.deleting'), successMessage: true } }
	)

// ---- Roles ----------------------------------------------------------------

export const getRoleList = (params: {
	page: number
	pageSize: number
	name?: string
	status?: number
}) => instance.get('/role/list', { params })

export const getRoleDetail = (roleId: number) =>
	instance.get('/role/detail', { params: { roleId } })

export const createRole = (params: {
	name: string
	description: string
	permissionIds: number[]
	status: number
}) =>
	instance.post('/role/create', params, {
		fetchOptions: { loading: t('rbac.api.loading.saving'), successMessage: true },
	})

export const updateRole = (params: {
	roleId: number
	name: string
	description: string
	permissionIds: number[]
	status: number
}) =>
	instance.post('/role/update', params, {
		fetchOptions: { loading: t('rbac.api.loading.saving'), successMessage: true },
	})

export const deleteRole = (roleId: number) =>
	instance.post(
		'/role/delete',
		{ roleId },
		{ fetchOptions: { loading: t('rbac.api.loading.deleting'), successMessage: true } }
	)

// ---- Permissions (module catalogue) --------------------------------------

export const getPermissionList = () =>
	instance.get('/permission/list', { params: { page: 1, pageSize: 1000 } })
