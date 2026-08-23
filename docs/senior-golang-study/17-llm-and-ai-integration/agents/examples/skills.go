package agentloop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxSkillBody ограничивает размер тела навыка. Навык подгружается в контекст
// целиком, поэтому файл на сотню килобайт стоит дороже, чем всё, что он
// экономит.
const maxSkillBody = 32 << 10

// Skill — одна папка с навыком. Наружу отдаются только имя и описание: они
// висят в контексте постоянно. Тело читается по запросу модели.
type Skill struct {
	Name        string
	Description string

	dir string // абсолютный путь к папке навыка
}

// Catalog — реестр навыков, найденных в корневой папке. Устройство повторяет
// постепенное раскрытие: каталог знает про все навыки, но в системный промпт
// отдаёт только описания.
type Catalog struct {
	root   string
	skills map[string]Skill
}

// LoadCatalog читает корневую папку и разбирает SKILL.md в каждой вложенной
// папке. Папка без SKILL.md пропускается: в дереве навыков часто лежат
// вспомогательные каталоги.
func LoadCatalog(root string) (*Catalog, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read skills root: %w", err)
	}

	c := &Catalog{root: root, skills: make(map[string]Skill)}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		s, err := loadSkill(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("skill %q: %w", e.Name(), err)
		}
		// Имя из фронтматтера может расходиться с именем папки. Ключом служит
		// имя из файла: именно его модель напишет в аргументах вызова.
		c.skills[s.Name] = s
	}
	return c, nil
}

// loadSkill разбирает SKILL.md одной папки.
func loadSkill(dir string) (Skill, error) {
	f, err := os.Open(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return Skill{}, err
	}
	defer f.Close()

	name, description, err := parseFrontMatter(f)
	if err != nil {
		return Skill{}, err
	}
	if name == "" || description == "" {
		return Skill{}, fmt.Errorf("front matter must define name and description")
	}
	return Skill{Name: name, Description: description, dir: dir}, nil
}

// parseFrontMatter читает YAML-заголовок между двумя строками "---".
// Полноценный парсер YAML здесь не нужен: интересуют два скалярных поля.
func parseFrontMatter(f *os.File) (name, description string, err error) {
	sc := bufio.NewScanner(f)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return "", "", fmt.Errorf("file must start with front matter")
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			return name, description, sc.Err()
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	return "", "", fmt.Errorf("front matter is not closed")
}

// SystemPromptSection — первый уровень раскрытия. Единственная часть навыков,
// которая присутствует в каждом запросе, поэтому длина описаний прямо
// переводится в стоимость каждого вызова.
func (c *Catalog) SystemPromptSection() string {
	if len(c.skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available skills. If a task matches a description, ")
	b.WriteString("load the skill with load_skill first.\n\n")
	for _, name := range c.names() {
		fmt.Fprintf(&b, "- %s: %s\n", name, c.skills[name].Description)
	}
	return b.String()
}

// names возвращает имена в стабильном порядке. Порядок важен не для чтения:
// перестановка строк меняет байты префикса запроса и обнуляет кэш промпта.
func (c *Catalog) names() []string {
	out := make([]string, 0, len(c.skills))
	for name := range c.skills {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Tool объявляет load_skill. Имена навыков перечислены в enum: модель не может
// запросить то, чего нет, и опечатка отсекается схемой до вызова.
func (c *Catalog) Tool() Tool {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
				"enum": c.names(),
			},
		},
		"required":             []string{"name"},
		"additionalProperties": false,
	}
	raw, _ := json.Marshal(schema)
	return Tool{
		Name:        "load_skill",
		Description: "Loads the full instruction of a skill by its name.",
		InputSchema: raw,
	}
}

// ToolFunc — второй уровень раскрытия: тело SKILL.md попадает в контекст
// только после явной заявки модели.
func (c *Catalog) ToolFunc() ToolFunc {
	return func(_ context.Context, args json.RawMessage) (string, error) {
		var in struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		s, ok := c.skills[in.Name]
		if !ok {
			// Текст ошибки уходит модели, поэтому он перечисляет допустимые
			// значения: со списком модель исправится за один шаг.
			return "", fmt.Errorf("unknown skill %q, available: %s", in.Name, strings.Join(c.names(), ", "))
		}
		return readCapped(filepath.Join(s.dir, "SKILL.md"))
	}
}

// ReadFile — третий уровень раскрытия: файлы, на которые ссылается тело навыка.
// Путь приходит от модели, а значит проверяется как недоверенный ввод.
func (c *Catalog) ReadFile(skillName, relPath string) (string, error) {
	s, ok := c.skills[skillName]
	if !ok {
		return "", fmt.Errorf("unknown skill %q", skillName)
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("path must be relative")
	}
	// Clean схлопывает "..", после чего достаточно проверить, что результат
	// остался внутри папки навыка. Без этой проверки "../../etc/passwd"
	// прочитается штатно.
	full := filepath.Join(s.dir, filepath.Clean("/"+relPath))
	if !strings.HasPrefix(full, s.dir+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes skill directory")
	}
	return readCapped(full)
}

// readCapped читает файл и обрезает его по лимиту. Обрезка помечается в тексте:
// молча укороченная инструкция хуже отсутствующей, потому что модель считает
// её полной.
func readCapped(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read skill file: %w", err)
	}
	if len(data) > maxSkillBody {
		return string(data[:maxSkillBody]) + "\n\n[truncated: skill body exceeds limit]", nil
	}
	return string(data), nil
}
