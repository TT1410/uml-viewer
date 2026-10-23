(ns uml-viewer.go-language.source-go
  "Go LanguageSource. A class is a package, so a member may live in any
  non-test file of the package directory; `locate` finds the one that
  declares it. `Type.Method` names a method on that receiver type."
  (:require [clojure.java.io :as io]
            [clojure.string :as str]
            [uml-viewer.source :as source])
  (:import [java.util.regex Pattern]))

(def ^:private plain-fmts
  ["(?m)^func\\s+%s\\s*[\\[(]"
   "(?m)^type\\s+%s\\b"
   "(?m)^\\s+%s\\s+(?:=\\s*)?(?:interface|struct|func|\\S)"
   "(?m)^(?:var|const)\\s+%s\\b"])

(defn- method-pattern [member-name]
  (let [[recv method] (str/split (str member-name) #"\." 2)]
    (when (and recv method)
      (re-pattern (str "(?m)^func\\s*\\([^)]*\\b" (Pattern/quote recv)
                       "(?:\\[[^\\]]*\\])?\\s*\\)\\s*" (Pattern/quote method) "\\s*[\\[(]")))))

(defn- start-of [text member-name]
  (when (and text member-name)
    (if-let [p (method-pattern member-name)]
      (let [m (re-matcher p text)]
        (when (.find m) (.start m)))
      (source/pattern-start text member-name plain-fmts))))

(defn member-line
  "1-based line of `member-name` in `text`, or nil."
  [text member-name]
  (when-let [start (start-of text member-name)]
    (source/line-number text start)))

(defn extract-member
  "Declaration line of `member-name`, or nil."
  [text member-name]
  (when-let [start (start-of text member-name)]
    (source/declaration-line text start)))

(defn- package-files [file]
  (let [dir (.getParentFile (.getCanonicalFile (io/file file)))]
    (->> (.listFiles dir)
         (filter #(let [n (.getName ^java.io.File %)]
                    (and (str/ends-with? n ".go") (not (str/ends-with? n "_test.go")))))
         (sort-by #(.getName ^java.io.File %)))))

(defn locate-member
  "Path of the package file that declares `(:name ident)`; the class file
  when no name is given or none declares it; nil when `:file` is missing."
  [ident]
  (when-let [f (source/existing-file ident)]
    (if-let [nm (not-empty (str (:name ident)))]
      (or (some (fn [g] (when (start-of (slurp g) nm)
                          (str/replace (.getPath ^java.io.File g) #"\\" "/")))
                (package-files f))
          f)
      f)))

(def impl
  (reify source/LanguageSource
    (locate [_ ident] (locate-member ident))
    (extract [_ text ident] (extract-member text (:name ident)))
    (start-line [_ text ident] (member-line text (:name ident)))
    (title [_ ident] (source/file-title ident))))

(source/register! :go impl)
