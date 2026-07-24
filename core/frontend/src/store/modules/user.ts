import { defineStore } from 'pinia'
import { confirm } from '@/utils'
import { logout as logoutApi, getCurrentUser } from '@/api/modules/user'
import i18n from '@/i18n'
import router from '@/router'

export default defineStore(
	'UserStore',
	() => {
		const { t } = i18n.global

		const login = ref({
			token: '', // Token
			refresh_token: '', // Refresh Token
			ttl: 0, // Token valid time/second
			expire: 0, // Token expiration time
		})

		// Current user profile + access control state.
		const profile = ref({
			id: 0,
			username: '',
			email: '',
		})
		// Role names held by the user (e.g. ['admin']).
		const roles = ref<string[]>([])
		// Logical module keys the user may access (menu/page gating).
		const permissions = ref<string[]>([])

		/**
		 * @description Determine if the user is logged in
		 */
		const isLogin = computed(() => {
			return login.value.token && login.value.expire > Date.now()
		})

		/**
		 * @description Whether the user holds the admin role (full access)
		 */
		const isAdmin = computed(() => roles.value.includes('admin'))

		/**
		 * @description Whether the user may access a logical module
		 * @param key module key, e.g. 'mailboxes'
		 */
		const hasModule = (key: string) => isAdmin.value || permissions.value.includes(key)

		/**
		 * @description Set user login information
		 * @param userVal
		 */
		const setLoginInfo = (userVal: { token: string; refresh_token: string; ttl: number }) => {
			login.value.token = userVal.token
			login.value.refresh_token = userVal.refresh_token
			login.value.ttl = userVal.ttl
			login.value.expire = userVal.ttl * 1000 + Date.now()
		}

		/**
		 * @description Store current-user access control info
		 */
		const setUserInfo = (data: {
			account?: { id: number; username: string; email: string }
			roles?: string[]
			permissions?: string[]
		}) => {
			if (data.account) {
				profile.value.id = data.account.id
				profile.value.username = data.account.username
				profile.value.email = data.account.email
			}
			roles.value = data.roles || []
			permissions.value = data.permissions || []
		}

		/**
		 * @description Fetch the current user's roles/permissions from the server.
		 * Safe to call whenever logged in; failures are swallowed so a transient
		 * error never blocks navigation.
		 */
		const fetchCurrentUser = async () => {
			try {
				const res = await getCurrentUser()
				if (res && typeof res === 'object') {
					setUserInfo(res as any)
				}
			} catch {
				// ignore — menu simply stays as last known state
			}
		}

		const resetLoginInfo = () => {
			login.value.token = ''
			login.value.refresh_token = ''
			login.value.ttl = 0
			login.value.expire = 0
			profile.value = { id: 0, username: '', email: '' }
			roles.value = []
			permissions.value = []
		}

		const logout = () => {
			confirm({
				title: t('user.logout.title'),
				content: t('user.logout.content'),
				onConfirm: async () => {
					await logoutApi()
					resetLoginInfo()
					router.push('/login')
				},
			})
		}

		return {
			login,
			profile,
			roles,
			permissions,
			isLogin,
			isAdmin,
			hasModule,
			logout,
			setLoginInfo,
			setUserInfo,
			fetchCurrentUser,
			resetLoginInfo,
		}
	},
	{
		persist: {
			pick: ['login', 'roles', 'permissions', 'profile'],
		},
	}
)
