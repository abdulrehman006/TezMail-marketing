export interface AccountParams {
	page: number
	page_size: number
	username: string
}

export interface Role {
	id: number
	name: string
	description: string
	status: number
	create_time: number
}

export interface Account {
	id: number
	username: string
	email: string
	status: number
	language: string
	create_time: number
}
