## Description

Please provide a brief summary of the changes introduced in this pull request and the motivation behind them.

Fixes #(issue)

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Internationalization (i18n translation addition or correction)

## Quality & Compliance Checklist

- [ ] My code follows the architectural guidelines and security mandates in [AGENTS.md](AGENTS.md).
- [ ] All database queries use GORM parameterized queries (zero SQL string concatenation).
- [ ] Zero AI unicode emojis used in code, commit messages, or UI copy.
- [ ] I have verified both English (`ui/src/locales/en.json`) and Vietnamese (`ui/src/locales/vi.json`) translation parity for any new UI strings.
- [ ] All tests pass locally (`go test -v ./...`).
- [ ] Frontend builds cleanly with zero errors (`cd ui && npm run build`).
