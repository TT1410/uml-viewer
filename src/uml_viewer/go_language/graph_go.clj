(ns uml-viewer.go-language.graph-go
  "Go LanguageGraph: one class per package (directory), import edges.
  The module path comes from go.mod. A project import is a dependency.
  A standard-library import is ignored. Any other import is foreign,
  named by its repository (`github.com/jackc/pgx/v5/pgxpool` is `jackc.pgx`)."
  (:require [clojure.java.io :as io]
            [clojure.string :as str]
            [uml-viewer.graph :as graph]))

(defn- canonical [file]
  (.getCanonicalFile (io/file file)))

(defn- go-mod-file
  "go.mod in `root` or the nearest directory above it, or nil."
  [root]
  (loop [dir (canonical root)]
    (when dir
      (let [f (io/file dir "go.mod")]
        (if (.isFile f) f (recur (.getParentFile dir)))))))

(defn module-path
  "Module path declared by go.mod `text`, or nil."
  [text]
  (second (re-find #"(?m)^\s*module\s+\"?([^\s\"]+)\"?" text)))

(defn- raw-string-end [source i n]
  (let [j (str/index-of source "`" (inc i))]
    (if j (inc j) n)))

(defn- mask-go-step [source sb i n]
  (let [c (.charAt ^String source i)
        nxt (graph/next-char source i)]
    (cond
      (graph/line-comment? c nxt) (graph/mask-line-comment source sb i n)
      (graph/block-comment? c nxt) (graph/mask-block-comment source sb i n)
      (= c \") (:i (graph/copy-span sb source [] i (graph/scan-quoted source i \" n)))
      (= c \') (:i (graph/copy-span sb source [] i (graph/scan-quoted source i \' n)))
      (= c \`) (:i (graph/copy-span sb source [] i (raw-string-end source i n)))
      :else (do (.append ^StringBuilder sb c) (inc i)))))

(defn mask-comments
  "Comments become spaces; string and rune literals are kept as they are."
  [source]
  (let [n (count source)
        sb (StringBuilder. n)]
    (loop [i 0]
      (if (>= i n)
        (str sb)
        (recur (mask-go-step source sb i n))))))

(def ^:private import-spec
  #"(?m)^\s*(?:[A-Za-z_][A-Za-z0-9_]*\s+|[._]\s+)?\"([^\"]+)\"")

(defn- import-blocks [text]
  (concat
    (map second (re-seq #"(?s)\bimport\s*\((.*?)\)" text))
    (map second (re-seq #"(?m)^\s*import\s+((?:[A-Za-z_][A-Za-z0-9_]*\s+|[._]\s+)?\"[^\"]+\")" text))))

(defn imports-of
  "Import paths in Go `source`, in order, without duplicates."
  [source]
  (let [text (mask-comments source)]
    (->> (import-blocks text)
         (mapcat #(map second (re-seq import-spec %)))
         distinct
         vec)))

(defn- exported? [name]
  (Character/isUpperCase (.charAt ^String name 0)))

(defn receiver-type
  "Type name of a method receiver such as `(s *Service)` or `(l List[T])`."
  [recv]
  (some->> (-> recv
               (str/replace #"^\(\s*" "")
               (str/replace #"\s*\)$" "")
               (str/replace #"\[[^\]]*\]" "")
               (str/replace #"\*" " ")
               str/trim)
           (re-find #"([A-Za-z_][A-Za-z0-9_]*)$")
           second))

(defn- funcs
  "Functions and methods. A method is `Type.Method`; it is exported only
  when both the receiver type and the method are."
  [text]
  (for [[_ recv name] (re-seq #"(?m)^func\s*(\([^)]*\))?\s*([A-Za-z_][A-Za-z0-9_]*)\s*[\[(]" text)]
    (if recv
      (let [t (receiver-type recv)]
        {:name (str t "." name) :exported (and (exported? t) (exported? name))})
      {:name name :exported (exported? name)})))

(def ^:private type-spec
  #"(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)(?:\s*\[[^\]]*\])?\s+(?:=\s*)?(interface|struct|func|\S+)")

(defn- types [text]
  (concat
    (for [[_ name kind] (re-seq #"(?m)^type\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*\[[^\]]*\])?\s+(?:=\s*)?(interface|struct|func|\S+)" text)
          :when (not= name "(")]
      {:name name :kind kind})
    (for [[_ block] (re-seq #"(?s)\ntype\s*\((.*?)\n\)" (str "\n" text))
          [_ name kind] (re-seq type-spec block)]
      {:name name :kind kind})))

(defn read-file
  "Surface of one Go `source` file: package name, imports, exported ops, types."
  [source]
  (let [text (mask-comments source)
        fs (funcs text)]
    {:package (second (re-find #"(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_]*)" text))
     :imports (imports-of source)
     :ops (->> fs (filter :exported) (map :name) vec)
     :types (vec (filter #(exported? (:name %)) (types text)))}))

(defn- go-source? [^java.io.File f]
  (let [n (.getName f)]
    (and (.isFile f)
         (str/ends-with? n ".go")
         (not (str/ends-with? n "_test.go")))))

(def ^:private skipped-dirs #{"vendor" "testdata" "node_modules" "target" "tmp"})

(defn- skipped-dir? [^java.io.File d]
  (let [n (.getName d)]
    (or (contains? skipped-dirs n)
        (str/starts-with? n ".")
        (str/starts-with? n "_"))))

(defn package-dirs
  "Directories under `root` that hold non-test Go files, sorted."
  [root]
  (loop [queue [(canonical root)] acc []]
    (if (empty? queue)
      (vec (sort-by #(.getPath ^java.io.File %) acc))
      (let [dir (first queue)
            kids (vec (or (.listFiles ^java.io.File dir) []))
            subdirs (filter #(and (.isDirectory ^java.io.File %) (not (skipped-dir? %))) kids)]
        (recur (into (vec (rest queue)) subdirs)
               (cond-> acc (some go-source? kids) (conj dir)))))))

(defn- rel-path [root dir]
  (str/replace (str (.relativize (.toPath (canonical root)) (.toPath (canonical dir))))
               #"\\" "/"))

(defn- import-path [module mod-root dir]
  (let [rel (rel-path mod-root dir)]
    (if (str/blank? rel) module (str module "/" rel))))

(defn- dotted [path]
  (str/replace path "/" "."))

(defn stdlib?
  "True for a standard-library import: its first segment has no dot."
  [path]
  (not (str/includes? (first (str/split path #"/")) ".")))

(defn foreign-id
  "Repository-level id of an outside import: owner.repo for a hosted path,
  otherwise the host. Major-version segments (`v5`) are dropped."
  [path]
  (let [[host & more] (remove #(re-matches #"v\d+" %) (str/split path #"/"))
        tail (vec more)]
    (keyword (str/join "." (if (seq tail) (take 2 tail) [host])))))

(defn- interface-only? [surfaces]
  (let [types (mapcat :types surfaces)]
    (and (seq types)
         (empty? (mapcat :ops surfaces))
         (every? #(= "interface" (:kind %)) types))))

(defn- package-class [dir files module mod-root prefix]
  (let [surfaces (mapv #(read-file (slurp %)) files)
        path (import-path module mod-root dir)
        ns-str (dotted path)
        id (graph/id-of ns-str prefix)
        ops (vec (distinct (concat (map :name (mapcat :types surfaces))
                                   (mapcat :ops surfaces))))]
    (cond-> {:id id
             :ns ns-str
             :name (graph/module-name id)
             :lang :go
             :file (graph/relative-path (first files))
             :path path
             :imports (vec (distinct (mapcat :imports surfaces)))}
      (seq ops) (assoc :ops (mapv (fn [n] {:name n :text n}) ops))
      (interface-only? surfaces) (assoc :stereotype :interface))))

(defn- dir-files [dir]
  (->> (.listFiles ^java.io.File dir)
       (filter go-source?)
       (sort-by #(.getName ^java.io.File %))
       vec))

(defn- in-module? [module imp]
  (or (= imp module) (str/starts-with? imp (str module "/"))))

(defn- edges-of [classes module]
  (let [by-path (into {} (map (juxt :path :id) classes))]
    (for [c classes
          imp (:imports c)
          :let [to (cond
                     (in-module? module imp) (get by-path imp)
                     (stdlib? imp) nil
                     :else (foreign-id imp))]
          :when (and to (not= to (:id c)))]
      {:from (:id c) :to to :kind :dependency :foreign (not (in-module? module imp))})))

(defrecord GoGraph []
  graph/LanguageGraph
  (scan [_ root opts]
    (let [mod-file (or (go-mod-file root)
                       (throw (ex-info (str "no go.mod at or above " root) {:root (str root)})))
          module (or (module-path (slurp mod-file))
                     (throw (ex-info (str "no module line in " mod-file) {:file (str mod-file)})))
          mod-root (.getParentFile (canonical mod-file))
          prefix (or (:prefix opts) (dotted module))
          classes (->> (package-dirs root)
                       (keep (fn [d] (let [fs (dir-files d)]
                                       (when (seq fs) (package-class d fs module mod-root prefix)))))
                       vec)
          edges (vec (distinct (edges-of classes module)))
          foreigns (->> edges (filter :foreign) (map :to) distinct (mapv graph/foreign-class))]
      {:classes (into (mapv #(dissoc % :path :imports) classes) foreigns)
       :edges (mapv #(dissoc % :foreign) edges)})))

(def impl (->GoGraph))

(graph/register! :go impl)
