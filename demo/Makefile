.PHONY: ci test

ci: test

test:
	@! rg -n 'tauth\.js|demoapi|wallet|secret_key' docker-compose.yml configs/config.yml README.md
	bash -n up.sh down.sh
	LEDGER_LOCAL_LEDGER_ENV_FILE=/dev/null LEDGER_LOCAL_TAUTH_ENV_FILE=/dev/null docker compose --project-name ledger-local config --quiet
