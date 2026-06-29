package embed

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ModelRepo é o repositório HuggingFace do modelo.
const ModelRepo = "Xenova/multilingual-e5-small"

// ModelFiles são os arquivos necessários do modelo.
var ModelFiles = []struct{ Name, URL string }{
	{
		Name: "model_quantized.onnx",
		URL:  "https://huggingface.co/Xenova/multilingual-e5-small/resolve/main/onnx/model_quantized.onnx",
	},
	{
		Name: "tokenizer.json",
		URL:  "https://huggingface.co/Xenova/multilingual-e5-small/resolve/main/tokenizer.json",
	},
}

// EnsureModel baixa os arquivos do modelo para modelDir se não existirem.
func EnsureModel(modelDir string) error {
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("criar diretório do modelo: %w", err)
	}
	for _, f := range ModelFiles {
		path := filepath.Join(modelDir, f.Name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := downloadFile(path, f.URL); err != nil {
			return fmt.Errorf("baixar %s: %w", f.Name, err)
		}
	}
	return nil
}

func downloadFile(dst, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
