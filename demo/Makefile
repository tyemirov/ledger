.PHONY: ci test

ci: test

test:
	@! rg -n 'tauth\.js|demoapi|wallet|secret_key' docker-compose.yml configs/config.yml README.md
	docker compose --profile localhost config --quiet
