package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {

	// func Rename(oldpath, newpath string) error

	cmdProject := flag.String("project", "test_project", "Указываем папку проекта")
	cmdTask := flag.Int("task", 0, "Указываем номер задачи по проекту")
	cmdTest := flag.Bool("test", false, "Брать тестовые данные")

	flag.Parse()

	fmt.Println("-----------------------------------------------------------------------------")
	fmt.Println("Пытаемся разобрать изменения в проекте", *cmdProject, "по задаче", *cmdTask)

	if err := os.Chdir(*cmdProject); err != nil { //Меняем директорию на рабочую или паникуем
		panic("Рабочая директория " + *cmdProject + " не найдена")
		// panic(err)
	}

	//В папке проекта нас должен ждать файл с настройками
	settingsFileName := filepath.Join(".go-deploy-config.json")
	settingsFileData, err := os.ReadFile(settingsFileName)
	checkFile(err)

	var settingsValues settings

	if err := json.Unmarshal(settingsFileData, &settingsValues); err != nil {
		panic(err)
	}
	if !settingsValues.IsActive {
		fmt.Println("Проект не активен")
		os.Exit(-1)
	}

	var gitText []byte
	if !*cmdTest {
		gitText, err = exec.Command("git", "log", "--pretty=oneline").CombinedOutput()

		if err != nil { //Ошибка выполнения команды
			// panic("Ошибка при выполнении команды " + cmd.String())
			panic(err)
		}
	} else {
		gitText, err = os.ReadFile("test_gitlog.txt")
		checkFile(err)
	}

	reg := regexp.MustCompile(`(?mU)^(\w+)\s.*(` + fmt.Sprintf("%d", *cmdTask) + `):?.*$`)

	affectedFiles := make(map[string][]string)
	for _, match := range reg.FindAllStringSubmatch(string(gitText), -1) {
		if match[1] != "" {
			files := make([]string, 0)
			var gitFilesText []byte
			if !*cmdTest {
				gitFilesText, err = exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", match[1]).CombinedOutput()

				if err != nil { //Ошибка выполнения команды
					// panic("Ошибка при выполнении команды " + cmd.String())
					panic(err)
				}
			} else {
				gitFilesText, err = os.ReadFile(match[1] + ".txt")
				checkFile(err)
			}

			lines := strings.Split(string(gitFilesText), "\n")

			for i := range lines {
				files = append(files, strings.Trim(lines[i], "\n\r "))
			}

			affectedFiles[match[1]] = files
		}
	}
	if 0 == len(affectedFiles) {
		fmt.Println("Коммитов по задаче не найдено")
		os.Exit(1)
	}
	deployAffected(affectedFiles, settingsValues)

}

func deployAffected(affectedFiles map[string][]string, settingsValues settings) {
	for key := range affectedFiles {
		fmt.Println("Разбираем коммит " + key)
		for i := range affectedFiles[key] {
			deployFile(affectedFiles[key][i], settingsValues)
		}
	}
}

func deployFile(affectedFile string, settingsValues settings) {
	dir, file := filepath.Split(affectedFile)
	targetFolder := filepath.Join(settingsValues.TargetPath, dir)
	mustRemove := false
	mustCreate := false

	targetFileName := filepath.Join(targetFolder, file)

	// fmt.Println(targetFileName)

	info, err := os.Lstat(affectedFile)
	if err != nil {
		fmt.Println("Ошибка получения информации о файле " + affectedFile + ". Удаляем целевой файл")
		mustRemove = true
	}

	// fmt.Println("info:Name", info.Name())
	// fmt.Println("info:Size", info.Size())
	// fmt.Println("info:Mode", info.Mode().Perm())
	// fmt.Printf("permissions: %#o\n", info.Mode().Perm()) // 0400, 0777, etc.
	// fmt.Println("info:ModTime", info.ModTime())
	// fmt.Println("info:ModTime", info.ModTime().Unix())
	// fmt.Println("info:IsDir", info.IsDir())
	// fmt.Println("info:Sys", info.Sys())
	// fmt.Println("--------------------------------")

	infoTarget, err := os.Lstat(targetFileName)

	if err != nil {
		if mustRemove {
			fmt.Println("Целевой файл " + targetFileName + " Отсутствует. Продолжаем")
			return
		}
		fmt.Println("Ошибка получения информации о файле " + targetFileName + ". Возможно это знак его создать")
		mustCreate = true
	}

	if mustRemove || mustCreate || info.ModTime().Unix() > infoTarget.ModTime().Unix() {
		if !mustCreate {
			err := os.Rename(targetFileName, targetFileName+".bak")
			if err != nil {
				fmt.Println("Ошибка создания резервной копии файла " + targetFileName)
			}
		}
		if !mustRemove { //Переименовали старый, надо создать новый. Если надо

			folderInfo, err := os.Lstat(dir)
			if err != nil {
				fmt.Println("Ошибка получения информации о папке файла " + dir)
			}

			targetFolderInfo, err := os.Lstat(targetFolder)
			if err != nil || !targetFolderInfo.IsDir() {
				os.MkdirAll(targetFolder, folderInfo.Mode().Perm())
			}

			e := copyFile(affectedFile, targetFileName)
			if e != nil {
				panic(e)
			}
		}
	} else {
		fmt.Println("Целевой файл " + targetFileName + " новее исходного. При необходимости перенесите руками")
	}

	// os.Exit(8)
}

// копируем файл
func copyFile(src string, dst string) (err error) {
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
		panic(e)
	}
}

type settings struct {
	IsActive     bool   `json:"isActive"`
	TargetPath   string `json:"targetPath"`
	UseMaker     bool   `json:"useMaker"`
	SrcFilesPath string `json:"srcFilesPath"`
	JsMakedPath  string `json:"jsMakedPath"`
	CssMakedPath string `json:"cssMakedPath"`
}
