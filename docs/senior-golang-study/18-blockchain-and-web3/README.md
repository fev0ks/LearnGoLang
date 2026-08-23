# Blockchain и Web3

Раздел даёт первое рабочее понимание blockchain и Web3 с позиции backend-разработчика. Здесь нет криптографической математики, внутреннего устройства виртуальных машин и подробного сравнения consensus-алгоритмов. Цель первого прохода — понимать термины, видеть общий поток транзакции и представлять, где в такой системе находится обычный Go-сервис.

## Материалы

1. [Blockchain и децентрализация](./01-blockchain-and-decentralization.md) — какую проблему решает blockchain, что хранят узлы и зачем сети нужен consensus.
2. [Wallet, транзакции и smart contracts](./02-wallets-transactions-and-smart-contracts.md) — чем wallet отличается от account, что подписывает пользователь и как выполняется контракт.
3. [Экосистема Web3](./03-web3-ecosystem.md) — dApp, токены, DeFi, DAO, oracle, bridge, L1 и L2 без углубления в их внутреннее устройство.
4. [Blockchain глазами backend-разработчика](./04-backend-developer-perspective.md) — чтение сети, отправка транзакций, индексирование событий и место Go-сервиса в архитектуре.

---

## Порядок чтения

Файлы идут от общей модели к практической интеграции. Для первого знакомства достаточно читать их по порядку и проверять, получается ли своими словами ответить на вопросы в блоках `Interview-ready answer`.

После первого прохода должно быть понятно:

- чем blockchain отличается от обычной базы данных;
- что означает децентрализация и почему она не бывает бесплатной;
- чем account, address, wallet и private key отличаются друг от друга;
- как транзакция попадает в сеть и почему результат появляется не мгновенно;
- что делает smart contract и чем он отличается от обычного backend-сервиса;
- почему dApp всё равно может содержать обычный frontend, backend и базу данных;
- какие задачи в blockchain-проекте обычно решает Go-разработчик.

---

## Граница первого прохода

Пока не требуется подробно изучать:

- устройство Ethereum Virtual Machine и отдельные инструкции;
- математические детали хеширования и цифровых подписей;
- доказательства корректности Proof of Work, Proof of Stake и BFT-протоколов;
- внутреннее устройство rollup, zero-knowledge proof и data availability;
- разработку production-контрактов на Solidity;
- Maximal Extractable Value, tokenomics и устройство конкретных DeFi-протоколов.

Эти темы полезны для специализации, но мешают сначала собрать цельную картину.

---

## Внешние источники

- [Ethereum: техническое введение](https://ethereum.org/developers/docs/intro-to-ethereum/)
- [Ethereum: введение в smart contracts](https://ethereum.org/developers/docs/smart-contracts/)
- [Ethereum development documentation](https://ethereum.org/developers/docs/)
- [Solidity documentation](https://docs.soliditylang.org/)
- [Go Ethereum](https://geth.ethereum.org/docs/developers/dapp-developer/native)
