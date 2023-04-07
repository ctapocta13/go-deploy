package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var settings settingsType

func main() {

	cmdProject := flag.String("project", "test_project", "Указываем папку проекта")
	cmdTask := flag.Int("task", 0, "Указываем номер задачи по проекту")
	cmdTest := flag.Bool("test", false, "Брать тестовые данные")
	debug := flag.Bool("debug", false, "Отладка")

	flag.Parse()

	fmt.Println("-----------------------------------------------------------------------------")
	if *cmdTask == 0 {
		fmt.Println("Укажите номер задачи")
		os.Exit(-1)
	}

	fmt.Println("Пытаемся разобрать изменения в проекте", *cmdProject, "по задаче", *cmdTask)

	if err := os.Chdir(*cmdProject); err != nil { //Меняем директорию на рабочую или выходим
		fmt.Println("Рабочая директория " + *cmdProject + " не найдена")
		os.Exit(-1)
	}

	err := settings.init(".go-deploy-config.json")

	checkErr(err)

	if *debug {
		settings.Verbose = true
	}

	if !settings.IsActive {
		fmt.Println("Проект не активен")
		os.Exit(-1)
	}

	affectedFiles, err := getCommitFiles(fmt.Sprintf("%d", *cmdTask), *cmdTest)
	checkErr(err)

	for key := range affectedFiles {
		deployFile(affectedFiles[key])
	}

	runMigartions(*cmdTask)
}

func checkErr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func runMigartions(task int) {
	ext, exist := settings.Migrations["type"]

	if !exist { //нет типа миграции
		return
	}

	if err := os.Chdir(settings.TargetPath); err != nil { //Меняем директорию на целевую
		if settings.Verbose {
			fmt.Println("Не могу перейти в целевую директорию " + settings.TargetPath)
		}
		return
	}

	if settings.Verbose {
		fmt.Println("Ищем миграцию по задаче", task)
	}
	taskNum := fmt.Sprint(task)

	migrationName := filepath.Join(".migrations", "migration-"+taskNum+"."+ext)

	_, err := os.Lstat(migrationName)
	if err != nil { //файла нет - делать нечего
		fmt.Println("Миграции нет")
		return
	}
	_, err_applied := os.Lstat(migrationName + ".applied")

	if err_applied == nil { //есть примененная миграция, удаляем миграцию и выходим
		fmt.Println("Миграция уже применена")
		os.Rename(migrationName, migrationName+".applied")
		return
	}

	os.Rename(migrationName, migrationName+".applied") //переименовываем чтобы не запускать второй раз

	cmdText, exist := settings.Migrations["command"]
	if !exist { //команды нет, выходим
		fmt.Println("Не задана команда миграции")
		return
	}
	cmdArgs, exist := settings.Migrations["command"]
	if !exist { //аргументов нет
		cmdArgs = ""
	}

	cmd := exec.Command(cmdText, cmdArgs)

	out, errorText := cmd.CombinedOutput()
	fmt.Println("Результат запуска миграции:", out)
	if errorText != nil {
		fmt.Println("Ошибки применения миграции:", errorText)
	}
}

func deployFile(affectedFile string) {
	fmt.Println("sleep 5 seconds")
	time.Sleep(5 * time.Second)
	fmt.Println("awake")
	dir, file := filepath.Split(affectedFile)
	if file == ".gitignore" || file == ".gitkeep" || file == ".go-deploy-config.json" {
		return
	}

	if settings.UseMaker { // Используется сборщик
		ext := strings.Trim(filepath.Ext(file), ".")

		rel, err := filepath.Rel(settings.SrcFilesPath, dir)

		if rel != "." && (rel[:2] != ".." || err != nil) { //папка вложена
			forType, exist := settings.MakedPath[ext] //exist показывает что настрока есть. если нет - можно не обрабатывать
			if !exist {
				return
			}
			deployFile(forType)
		}
	}

	targetFolder := filepath.Join(settings.TargetPath, dir)
	mustRemove := false
	mustCreate := false

	targetFileName := filepath.Join(targetFolder, file)

	info, err := os.Lstat(affectedFile)
	if err != nil {
		fmt.Println(targetFileName + " удаляем")
		mustRemove = true
	}

	infoTarget, err := os.Lstat(targetFileName)

	if err != nil {
		if mustRemove {
			fmt.Println(targetFileName + " отсутствует")
			return
		}
		fmt.Println(targetFileName + " создаем")
		mustCreate = true
	}

	if mustRemove || mustCreate || info.ModTime().Unix() > infoTarget.ModTime().Unix() {
		if !mustCreate {
			err := os.Rename(targetFileName, targetFileName+".bak")
			if err != nil {
				fmt.Println(targetFileName + "ошибка создания резервной копии файла ")
			}
		}
		if !mustRemove { //Переименовали старый, надо создать новый. Если надо

			folderInfo, err := os.Lstat(dir)
			if err != nil {
				fmt.Println(dir + "ошибка получения информации о папке файла ")
			}

			targetFolderInfo, err := os.Lstat(targetFolder)
			if err != nil || !targetFolderInfo.IsDir() {
				os.MkdirAll(targetFolder, folderInfo.Mode().Perm())
			}

			e := copyFile(affectedFile, targetFileName)
			if e != nil {
				fmt.Println("Ошибка копирования файла")
				fmt.Println(e)
				os.Exit(-1)
			}
		}
	} else {
		fmt.Println(targetFileName + " новее исходного. При необходимости перенесите руками")
	}
}

func copyFile(src string, dst string) (err error) { // копируем файл
	sourcefile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourcefile.Close()
	destfile, err := os.Create(dst)
	if err != nil {
		return err
	}
	//копируем содержимое и проверяем коды ошибок
	_, err = io.Copy(destfile, sourcefile)
	if closeErr := destfile.Close(); err == nil {
		//если ошибки в io.Copy нет, то берем ошибку от destfile.Close(), если она была
		err = closeErr
	}
	if err != nil {
		return err
	}
	sourceinfo, err := os.Stat(src)
	if err == nil {
		err = os.Chmod(dst, sourceinfo.Mode())
	}
	return err
}

func checkFile(e error) {
	if e != nil {
		fmt.Println("Ошибка работы с файлом")
		fmt.Println(e)
		os.Exit(-1)
	}
}

type settingsType struct {
	IsActive       bool              `json:"isActive"`
	TargetPath     string            `json:"targetPath"`
	UseMaker       bool              `json:"useMaker"`
	SrcFilesPath   string            `json:"srcFilesPath"`
	MakedPath      map[string]string `json:"makedPath"`
	Migrations     map[string]string `json:"migrations"`
	HaveMigraption bool
	Verbose        bool
}

func (settings *settingsType) init(filename string) error {
	settingsFileData, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(settingsFileData, &settings); err != nil {
		return errors.New("Ошибка разбора настроек")
	}

	settings.Verbose = false
	return nil
}

func getCommitFiles(task string, test bool) (map[string]string, error) {
	var gitText []byte
	var err error
	affectedFiles := make(map[string]string)

	if !test {
		gitText, err = exec.Command("git", "log", "--pretty=oneline").CombinedOutput()

		if err != nil { //Ошибка выполнения команды
			fmt.Println("Ошибка при получении списка коммитов. Проверьте наличие проекта и настройки git")
			return affectedFiles, err
		}
	} else {
		gitText, err = os.ReadFile("test_gitlog.txt")
		checkFile(err)
	}

	reg := regexp.MustCompile(`(?mU)^(\w+)\s.*(` + task + `):?\s.*$`)

	for _, match := range reg.FindAllStringSubmatch(string(gitText), -1) {
		if match[1] != "" {
			var gitFilesText []byte
			if !test {
				gitFilesText, err = exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", match[1]).CombinedOutput()

				if err != nil { //Ошибка выполнения команды
					return affectedFiles, err
				}
			} else {
				gitFilesText, err = os.ReadFile(match[1] + ".txt")
				checkFile(err)
			}

			lines := strings.Split(strings.Trim(string(gitFilesText), "\n\r"), "\n") //Вывод обрезаем для исключения пустой строки

			for i := range lines {
				file := strings.Trim(lines[i], "\n\r ")

				affectedFiles[file] = file
			}

		}
	}

	if 0 == len(affectedFiles) {
		err = errors.New("Изменений по задаче не найдено")
	}

	return affectedFiles, err
}
