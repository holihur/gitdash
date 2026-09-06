// Package api 提供 gitdash 的 REST API。
//
//	@title       gitdash API
//	@version     1.0
//	@description git 托管服务 REST API。除 /api/health、/api/version、/api/openapi.json 与认证端点外，均需 Bearer token（由注册 / 登录返回）。
//
//	@securityDefinitions.apikey BearerAuth
//	@in                         header
//	@name                       Authorization
//	@description                形如 "Bearer <token>"。
package api
