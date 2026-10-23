(ns uml-viewer.go-language.graph-go-spec
  (:require [clojure.java.io :as io]
            [speclj.core :refer :all]
            [uml-viewer.go-language.graph-go :as go]
            [uml-viewer.graph :as graph]))

(defn- spit-file [dir rel content]
  (let [f (io/file dir rel)]
    (io/make-parents f)
    (spit f content)
    f))

(defn- temp-module []
  (doto (io/file (System/getProperty "java.io.tmpdir")
                 (str "uml-go-" (System/nanoTime)))
    (.mkdirs)))

(describe "go module path"
  (it "reads the module line of go.mod"
    (should= "example.com/shop" (go/module-path "// c\nmodule example.com/shop\n\ngo 1.23\n"))
    (should-be-nil (go/module-path "go 1.23\n"))))

(describe "go file surface"
  (it "reads grouped and single imports, aliases, and blank imports"
    (should= ["context" "example.com/shop/internal/order" "github.com/jackc/pgx/v5" "embed"]
             (:imports (go/read-file
                         (str "package app\n\n"
                              "import (\n"
                              "\t\"context\"\n"
                              "\t// \"commented/out\"\n"
                              "\torder \"example.com/shop/internal/order\"\n"
                              "\t\"github.com/jackc/pgx/v5\"\n"
                              ")\n"
                              "import _ \"embed\"\n")))))

  (it "ignores import-like text inside strings and comments"
    (should= ["fmt"]
             (:imports (go/read-file
                         (str "package a\nimport \"fmt\"\n"
                              "/* import \"os\" */\n"
                              "var s = `import \"net\"`\n")))))

  (it "lists exported functions, methods on exported types, and exported types"
    (let [surface (go/read-file
                    (str "package order\n"
                         "type Service struct{}\n"
                         "type hidden struct{}\n"
                         "type (\n\tRepo interface{ Get() }\n\tID int\n)\n"
                         "func New() *Service { return nil }\n"
                         "func helper() {}\n"
                         "func (s *Service) Place(id ID) error { return nil }\n"
                         "func (s *Service) check() {}\n"
                         "func (h hidden) Visible() {}\n"
                         "func Map[T any](xs []T) []T { return xs }\n"
                         "func (l *List[T]) Push(x T) {}\n"))]
      (should= "order" (:package surface))
      (should= ["New" "Service.Place" "Map" "List.Push"] (:ops surface))
      (should= #{"Service" "Repo" "ID"} (set (map :name (:types surface))))))

  (it "names the receiver type of a method"
    (should= "Service" (go/receiver-type "(s *Service)"))
    (should= "List" (go/receiver-type "(l *List[T])"))
    (should= "Plain" (go/receiver-type "(Plain)"))))

(describe "go imports"
  (it "tells the standard library from everything else"
    (should (go/stdlib? "net/http"))
    (should-not (go/stdlib? "github.com/jackc/pgx/v5")))

  (it "names an outside import by its repository"
    (should= :jackc.pgx (go/foreign-id "github.com/jackc/pgx/v5/pgxpool"))
    (should= :x.text (go/foreign-id "golang.org/x/text/language"))
    (should= :yaml.v3 (go/foreign-id "gopkg.in/yaml.v3"))))

(describe "go graph"
  (it "scans packages, links project imports, and keeps outside libraries as foreign"
    (let [dir (temp-module)]
      (spit-file dir "go.mod" "module example.com/shop\n\ngo 1.23\n")
      (spit-file dir "internal/order/order.go"
                 (str "package order\n"
                      "import (\n\t\"context\"\n\t\"example.com/shop/internal/platform/db\"\n"
                      "\t\"github.com/jackc/pgx/v5/pgxpool\"\n)\n"
                      "type Service struct{ pool *pgxpool.Pool }\n"
                      "func (s *Service) Place(ctx context.Context) error { return db.Do(ctx) }\n"))
      (spit-file dir "internal/order/order_test.go"
                 "package order\nimport \"example.com/shop/internal/billing\"\n")
      (spit-file dir "internal/platform/db/db.go"
                 "package db\nimport \"context\"\nfunc Do(ctx context.Context) error { return nil }\n")
      (spit-file dir "internal/billing/port.go"
                 "package billing\ntype Charger interface{ Charge() error }\n")
      (spit-file dir "internal/testdata/x.go" "package testdata\n")
      (let [g (graph/scan (graph/lookup :go) (io/file dir "internal")
                          {:prefix "example.com.shop.internal"})
            by-id (into {} (map (juxt :id identity) (:classes g)))
            edges (set (map (juxt :from :to :kind) (:edges g)))]
        (should= #{:order :platform.db :billing :jackc.pgx} (set (keys by-id)))
        (should= "example.com.shop.internal.order" (:ns (by-id :order)))
        (should= :go (:lang (by-id :order)))
        (should= ["Service" "Service.Place"] (mapv :name (:ops (by-id :order))))
        (should= :interface (:stereotype (by-id :billing)))
        (should (:foreign (by-id :jackc.pgx)))
        (should= #{[:order :platform.db :dependency]
                   [:order :jackc.pgx :dependency]}
                 edges)))))
