(ns uml-viewer.go-language.source-go-spec
  (:require [clojure.java.io :as io]
            [speclj.core :refer :all]
            [uml-viewer.go-language.source-go :as go-source]
            [uml-viewer.source :as source]))

(def text
  (str "package order\n\n"
       "type Service struct{}\n\n"
       "type (\n\tID int\n)\n\n"
       "func New() *Service { return nil }\n\n"
       "func (s *Service) Place(id ID) error {\n\treturn nil\n}\n"))

(describe "go member lookup"
  (it "finds functions, types, grouped types, and methods by line"
    (should= 3 (go-source/member-line text "Service"))
    (should= 6 (go-source/member-line text "ID"))
    (should= 9 (go-source/member-line text "New"))
    (should= 11 (go-source/member-line text "Service.Place"))
    (should-be-nil (go-source/member-line text "Missing")))

  (it "extracts the declaration line"
    (should= "func (s *Service) Place(id ID) error {"
             (go-source/extract-member text "Service.Place"))))

(describe "go member files"
  (it "opens the package file that declares the member"
    (let [dir (doto (io/file (System/getProperty "java.io.tmpdir") (str "uml-go-src-" (System/nanoTime)))
                (.mkdirs))
          a (io/file dir "a.go")
          b (io/file dir "b.go")]
      (spit a "package order\n\nfunc New() {}\n")
      (spit b "package order\n\nfunc Other() {}\n")
      (spit (io/file dir "b_test.go") "package order\n\nfunc TestOther() {}\n")
      (let [found (source/member-source :go {:file (.getPath a) :name "Other" :lang :go})]
        (should (.endsWith (str (:file found)) "b.go"))
        (should= 3 (:line found)))
      (should-be-nil (source/member-source :go {:file (.getPath a) :name "TestOther" :lang :go})))))
