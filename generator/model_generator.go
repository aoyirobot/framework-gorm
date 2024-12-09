package generator

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func GenerateModel() {
	// Set the folder where your SQL files are located
	sqlFolder := "./database"
	modelFolder := "./internal/model"

	// Create the model folder if it doesn't exist
	if err := os.MkdirAll(modelFolder, os.ModePerm); err != nil {
		fmt.Println("Error creating model folder:", err)
		return
	}

	// Read all the SQL files in the folder
	files, err := os.ReadDir(sqlFolder)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sql") {
			// Process each SQL file
			sqlFilePath := fmt.Sprintf("%s/%s", sqlFolder, file.Name())

			// Generate Go model files for each table in the SQL file
			err := generateModelsFromSQL(sqlFilePath, modelFolder)
			if err != nil {
				fmt.Println("Error generating models for", file.Name(), ":", err)
			} else {
				fmt.Println("Generated models for", file.Name())
			}
		}
	}
}

func generateModelsFromSQL(sqlFilePath string, modelFolder string) error {
	// 打开SQL文件
	file, err := os.Open(sqlFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 逐行读取SQL文件
	scanner := bufio.NewScanner(file)

	var currentTableName string
	var modelContent strings.Builder
	var primaryKeyFields []string // 用于存储主键字段
	var tableColumns []string     // 用于存储所有列定义（包含主键）

	// 记录是否已添加主键字段
	primaryKeyAdded := false

	// 括号计数器
	openBrackets := 0
	closeBrackets := 0

	for scanner.Scan() {
		line := scanner.Text()

		// 跳过注释和空行
		if strings.HasPrefix(line, "--") || strings.TrimSpace(line) == "" {
			continue
		}

		// 处理CREATE TABLE语句，提取表名和列定义
		if strings.HasPrefix(line, "CREATE TABLE") {
			// 如果当前表正在处理中，则保存模型并开始处理下一个表
			if currentTableName != "" {
				// 保存当前表的模型到文件
				modelFilePath := fmt.Sprintf("%s/%s.go", modelFolder, strings.ToLower(toCamelCase(currentTableName)))
				modelContent.WriteString("}\n\n") // 确保每个结构体正确关闭
				if err := os.WriteFile(modelFilePath, []byte(modelContent.String()), 0644); err != nil {
					return err
				}
				// 重置模型内容，准备处理下一个表
				modelContent.Reset()
			}

			// 提取表名
			currentTableName = extractTableName(line)
			if currentTableName != "" {
				modelContent.WriteString(fmt.Sprintf("package model\n\n"))
				modelContent.WriteString(fmt.Sprintf("type %s struct {\n", strings.Title(toCamelCase(currentTableName))))
				primaryKeyFields = []string{} // 重置主键字段列表
				primaryKeyAdded = false       // 重置主键添加标志
				tableColumns = []string{}     // 重置列定义
			}
		}

		// 统计当前行的括号数量
		openBrackets = strings.Count(line, "(")
		closeBrackets = strings.Count(line, ")")

		// 如果当前表已开始处理，且括号匹配，处理列定义
		if currentTableName != "" && closeBrackets-openBrackets == 1 {
			// 第一次遍历处理主键字段
			processPrimaryKey(tableColumns, &modelContent, &primaryKeyFields, &primaryKeyAdded)

			// 第二次遍历处理其他列定义
			for _, column := range tableColumns {
				processColumn(column, &modelContent, primaryKeyFields, &primaryKeyAdded)
			}

			// 完成当前表的定义
			modelFilePath := fmt.Sprintf("%s/%s.go", modelFolder, strings.ToLower(toCamelCase(currentTableName)))
			modelContent.WriteString("}\n\n") // 确保结构体正确关闭
			if err := os.WriteFile(modelFilePath, []byte(modelContent.String()), 0644); err != nil {
				return err
			}
			currentTableName = "" // 重置以处理下一个表
			tableColumns = []string{}
			currentTableName = ""
			modelContent = strings.Builder{}
		} else if currentTableName != "" && closeBrackets-openBrackets != 1 {
			// 收集列定义
			tableColumns = append(tableColumns, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// 如果最后一个表没有写入文件（例如SQL没有以“);”结尾）
	if currentTableName != "" {
		modelFilePath := fmt.Sprintf("%s/%s.go", modelFolder, strings.ToLower(toCamelCase(currentTableName)))
		modelContent.WriteString("}\n\n") // 确保结构体正确关闭
		if err := os.WriteFile(modelFilePath, []byte(modelContent.String()), 0644); err != nil {
			return err
		}
	}

	return nil
}

// 提取表名
func extractTableName(line string) string {
	re := regexp.MustCompile(`CREATE TABLE ` + "`" + `([^` + "`" + `]+)` + "`")
	match := re.FindStringSubmatch(line)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// 处理列定义（包括字段类型、约束等）
func processColumn(line string, modelContent *strings.Builder, primaryKeyFields []string, primaryKeyAdded *bool) {
	// 清理行中多余的DESC和ASC
	line = strings.ReplaceAll(line, " DESC", "")
	line = strings.ReplaceAll(line, " ASC", "")

	// 提取列名和类型，检查是否有大小或其他约束
	re := regexp.MustCompile(`\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s+([a-zA-Z0-9]+)(\([0-9]+\))?\s*(not null|default\s*[^;]+|comment\s*'[^']*')?`)
	match := re.FindStringSubmatch(line)
	if len(match) < 3 {
		return
	}

	columnName := match[1]
	columnType := match[2]
	columnSize := match[3]        // 捕获大小，如(100)
	columnConstraints := match[4] // 捕获约束：not null, default, comment

	// 将SQL类型映射为Go类型
	goType := mapSQLTypeToGoType(columnType)

	// 转换列名为CamelCase，并确保首字母大写
	fieldName := toCamelCase(columnName)
	fieldName = strings.Title(fieldName) // 确保首字母大写

	// 初始化GORM标签
	gormTags := []string{"not null"} // 默认是"not null"，如果没有其他约束

	// 处理列大小，如果有（如varchar(100)）
	if columnSize != "" {
		// 提取括号中的数字并加到标签中（例如，size:100）
		size := strings.Trim(columnSize, "()")
		gormTags = append(gormTags, fmt.Sprintf("size:%s", size))
	}

	// 处理列的约束（DEFAULT，COMMENT等）
	if columnConstraints != "" {
		// 处理DEFAULT
		if strings.HasPrefix(columnConstraints, "default") {
			defaultValue := strings.TrimSpace(strings.TrimPrefix(columnConstraints, "default"))
			gormTags = append(gormTags, fmt.Sprintf("default:%s", defaultValue))
		}

		// 处理COMMENT
		if strings.HasPrefix(columnConstraints, "comment") {
			comment := strings.TrimSpace(strings.TrimPrefix(columnConstraints, "comment"))
			gormTags = append(gormTags, fmt.Sprintf("comment:'%s'", comment))
		}
	}

	// 生成GORM标签字符串
	_ = fmt.Sprintf("gorm:\"%s\"", strings.Join(gormTags, ";"))

	// 检查字段是否是主键，并添加相应的标签
	if contains(primaryKeyFields, columnName) && !*primaryKeyAdded {
		// 如果是主键字段，添加主键标签
		*primaryKeyAdded = true
		modelContent.WriteString(fmt.Sprintf("    %s %s `json:\"%s\" gorm:\"primaryKey;autoIncrement;%s\"`\n", fieldName, goType, fieldName, strings.Join(gormTags, ";")))
	} else {
		// 普通列字段添加标签
		modelContent.WriteString(fmt.Sprintf("    %s %s `json:\"%s\" gorm:\"%s\"`\n", fieldName, goType, fieldName, strings.Join(gormTags, ";")))
	}
}

// 处理主键字段
func processPrimaryKey(tableColumns []string, modelContent *strings.Builder, primaryKeyFields *[]string, primaryKeyAdded *bool) {
	for _, column := range tableColumns {
		// 清理行中多余的DESC和ASC
		column = strings.ReplaceAll(column, " DESC", "")
		column = strings.ReplaceAll(column, " ASC", "")
		// 检查是否包含PRIMARY KEY
		if strings.Contains(column, "PRIMARY KEY") {
			// 提取主键字段
			re := regexp.MustCompile(`PRIMARY KEY\s*\(([^)]+)\)`)
			match := re.FindStringSubmatch(column)
			if len(match) > 1 {
				columns := strings.Split(match[1], ",")
				for _, col := range columns {
					col = strings.TrimSpace(col)
					col = strings.Trim(col, "`")
					// 添加到主键字段列表
					*primaryKeyFields = append(*primaryKeyFields, col)
				}
			}
			break // 只处理一次PRIMARY KEY定义
		}
	}
}

// Map SQL types to Go types
func mapSQLTypeToGoType(sqlType string) string {
	switch sqlType {
	case "int", "integer", "tinyint":
		return "int"
	case "varchar", "text":
		return "string"
	case "double":
		return "float64"
	case "datetime":
		return "time.Time"
	case "bit":
		return "bool"
	case "decimal":
		return "decimal.Decimal"
	default:
		return "string" // Default fallback type
	}
}

func toCamelCase(s string) string {
	// Split the string by underscores and capitalize each part except the first one
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
