module Html exposing
  ( Html, Attribute
  , text, node, map
  , h1, h2, h3, h4, h5, h6
  , div, p, hr, pre, blockquote
  , span, a, code, em, strong, i, b, u, sub, sup, br
  , ol, ul, li, dl, dt, dd
  , img, iframe, canvas, math
  , form, input, textarea, button, select, option
  , section, nav, article, aside, header, footer, address, main_
  , figure, figcaption
  , table, caption, colgroup, col, tbody, thead, tfoot, tr, td, th
  , fieldset, legend, label, datalist, optgroup, output, progress, meter
  , audio, video, source, track
  , embed, object, param
  , ins, del
  , small, cite, dfn, abbr, time, var, samp, kbd, s, q
  , mark, ruby, rt, rp, bdi, bdo, wbr
  , details, summary, menuitem, menu
  )

{-| This file is organized roughly in order of popularity. The tags which you'd
expect to use frequently will be closer to the top.

# Primitives
@docs Html, Attribute, text, node, map

# Tags

## Headers
@docs h1, h2, h3, h4, h5, h6

## Grouping Content
@docs div, p, hr, pre, blockquote

## Text
@docs span, a, code, em, strong, i, b, u, sub, sup, br

## Lists
@docs ol, ul, li, dl, dt, dd

## Embedded Content
@docs img, iframe, canvas, math

## Inputs
@docs form, input, textarea, button, select, option

## Sections
@docs section, nav, article, aside, header, footer, address, main_

## Figures
@docs figure, figcaption

## Tables
@docs table, caption, colgroup, col, tbody, thead, tfoot, tr, td, th


## Less Common Elements

### Less Common Inputs
@docs fieldset, legend, label, datalist, optgroup, output, progress, meter

### Audio and Video
@docs audio, video, source, track

### Embedded Objects
@docs embed, object, param

### Text Edits
@docs ins, del

### Semantic Text
@docs small, cite, dfn, abbr, time, var, samp, kbd, s, q

### Less Common Text Tags
@docs mark, ruby, rt, rp, bdi, bdo, wbr

## Interactive Elements
@docs details, summary, menuitem, menu

-}


import Elm.Kernel.VirtualDom
import VirtualDom



-- CORE TYPES


{-| The core building block used to build up HTML. Here we create an `Html`
value with no attributes and one child:

    hello : Html msg
    hello =
      div [] [ text "Hello!" ]
-}
type alias Html msg = VirtualDom.Node msg


{-| Set attributes on your `Html`. Learn more in the
[`Html.Attributes`](Html-Attributes) module.
-}
type alias Attribute msg = VirtualDom.Attribute msg



-- PRIMITIVES


{-| General way to create HTML nodes. It is used to define all of the helper
functions in this library.

    div : List (Attribute msg) -> List (Html msg) -> Html msg
    div attributes children =
        node "div" attributes children

You can use this to create custom nodes if you need to create something that
is not covered by the helper functions in this library.
-}
node : String -> List (Attribute msg) -> List (Html msg) -> Html msg
node =
  VirtualDom.node


{-| Just put plain text in the DOM. It will escape the string so that it appears
exactly as you specify.

    text "Hello World!"
-}
text : String -> Html msg
text =
  VirtualDom.text



-- NESTING VIEWS


{-| Transform the messages produced by some `Html`. In the following example,
we have `viewButton` that produces `()` messages, and we transform those values
into `Msg` values in `view`.

    type Msg = Left | Right

    view : model -> Html Msg
    view model =
      div []
        [ map (\_ -> Left) (viewButton "Left")
        , map (\_ -> Right) (viewButton "Right")
        ]

    viewButton : String -> Html ()
    viewButton name =
      button [ onClick () ] [ text name ]

This should not come in handy too often. Definitely read [this][reuse] before
deciding if this is what you want.

[reuse]: https://guide.elm-lang.org/reuse/
-}
map : (a -> msg) -> Html a -> Html msg
map =
  VirtualDom.map



-- SECTIONS


{-| Defines a section in a document.
-}
section : List (Attribute msg) -> List (Html msg) -> Html msg
section =
  Elm.Kernel.VirtualDom.node "section"


{-| Defines a section that contains only navigation links.
-}
nav : List (Attribute msg) -> List (Html msg) -> Html msg
nav =
  Elm.Kernel.VirtualDom.node "nav"


{-| Defines self-contained content that could exist independently of the rest
of the content.
-}
article : List (Attribute msg) -> List (Html msg) -> Html msg
article =
  Elm.Kernel.VirtualDom.node "article"


{-| Defines some content loosely related to the page content. If it is removed,
the remaining content still makes sense.
-}
aside : List (Attribute msg) -> List (Html msg) -> Html msg
aside =
  Elm.Kernel.VirtualDom.node "aside"


{-|-}
h1 : List (Attribute msg) -> List (Html msg) -> Html msg
h1 =
  Elm.Kernel.VirtualDom.node "h1"


{-|-}
h2 : List (Attribute msg) -> List (Html msg) -> Html msg
h2 =
  Elm.Kernel.VirtualDom.node "h2"


{-|-}
h3 : List (Attribute msg) -> List (Html msg) -> Html msg
h3 =
  Elm.Kernel.VirtualDom.node "h3"


{-|-}
h4 : List (Attribute msg) -> List (Html msg) -> Html msg
h4 =
  Elm.Kernel.VirtualDom.node "h4"


{-|-}
h5 : List (Attribute msg) -> List (Html msg) -> Html msg
h5 =
  Elm.Kernel.VirtualDom.node "h5"


{-|-}
h6 : List (Attribute msg) -> List (Html msg) -> Html msg
h6 =
  Elm.Kernel.VirtualDom.node "h6"


{-| Defines the header of a page or section. It often contains a logo, the
title of the web site, and a navigational table of content.
-}
header : List (Attribute msg) -> List (Html msg) -> Html msg
header =
  Elm.Kernel.VirtualDom.node "header"


{-| Defines the footer for a page or section. It often contains a copyright
notice, some links to legal information, or addresses to give feedback.
-}
footer : List (Attribute msg) -> List (Html msg) -> Html msg
footer =
  Elm.Kernel.VirtualDom.node "footer"


{-| Defines a section containing contact information. -}
address : List (Attribute msg) -> List (Html msg) -> Html msg
address =
  Elm.Kernel.VirtualDom.node "address"


{-| Defines the main or important content in the document. There is only one
`main` element in the document.
-}
main_ : List (Attribute msg) -> List (Html msg) -> Html msg
main_ =
  Elm.Kernel.VirtualDom.node "main"


-- GROUPING CONTENT

{-| Defines a portion that should be displayed as a paragraph. -}
p : List (Attribute msg) -> List (Html msg) -> Html msg
p =
  Elm.Kernel.VirtualDom.node "p"


{-| Represents a thematic break between paragraphs of a section or article or
any longer content.
-}
hr : List (Attribute msg) -> List (Html msg) -> Html msg
hr =
  Elm.Kernel.VirtualDom.node "hr"


{-| Indicates that its content is preformatted and that this format must be
preserved.
-}
pre : List (Attribute msg) -> List (Html msg) -> Html msg
pre =
  Elm.Kernel.VirtualDom.node "pre"


{-| Represents a content that is quoted from another source. -}
blockquote : List (Attribute msg) -> List (Html msg) -> Html msg
blockquote =
  Elm.Kernel.VirtualDom.node "blockquote"


{-| Defines an ordered list of items. -}
ol : List (Attribute msg) -> List (Html msg) -> Html msg
ol =
  Elm.Kernel.VirtualDom.node "ol"


{-| Defines an unordered list of items. -}
ul : List (Attribute msg) -> List (Html msg) -> Html msg
ul =
  Elm.Kernel.VirtualDom.node "ul"


{-| Defines a item of an enumeration list. -}
li : List (Attribute msg) -> List (Html msg) -> Html msg
li =
  Elm.Kernel.VirtualDom.node "li"


{-| Defines a definition list, that is, a list of terms and their associated
definitions.
-}
dl : List (Attribute msg) -> List (Html msg) -> Html msg
dl =
  Elm.Kernel.VirtualDom.node "dl"


{-| Represents a term defined by the next `dd`. -}
dt : List (Attribute msg) -> List (Html msg) -> Html msg
dt =
  Elm.Kernel.VirtualDom.node "dt"


{-| Represents the definition of the terms immediately listed before it. -}
dd : List (Attribute msg) -> List (Html msg) -> Html msg
dd =
  Elm.Kernel.VirtualDom.node "dd"


{-| Represents a figure illustrated as part of the document. -}
figure : List (Attribute msg) -> List (Html msg) -> Html msg
figure =
  Elm.Kernel.VirtualDom.node "figure"


{-| Represents the legend of a figure. -}
figcaption : List (Attribute msg) -> List (Html msg) -> Html msg
figcaption =
  Elm.Kernel.VirtualDom.node "figcaption"


{-| Represents a generic container with no special meaning. -}
div : List (Attribute msg) -> List (Html msg) -> Html msg
div =
  Elm.Kernel.VirtualDom.node "div"


-- TEXT LEVEL SEMANTIC

{-| Represents a hyperlink, linking to another resource. -}
a : List (Attribute msg) -> List (Html msg) -> Html msg
a =
  Elm.Kernel.VirtualDom.node "a"


{-| Represents emphasized text, like a stress accent. -}
em : List (Attribute msg) -> List (Html msg) -> Html msg
em =
  Elm.Kernel.VirtualDom.node "em"


{-| Represents especially important text. -}
strong : List (Attribute msg) -> List (Html msg) -> Html msg
strong =
  Elm.Kernel.VirtualDom.node "strong"


{-| Represents a side comment, that is, text like a disclaimer or a
copyright, which is not essential to the comprehension of the document.
-}
small : List (Attribute msg) -> List (Html msg) -> Html msg
small =
  Elm.Kernel.VirtualDom.node "small"


{-| Represents content that is no longer accurate or relevant. -}
s : List (Attribute msg) -> List (Html msg) -> Html msg
s =
  Elm.Kernel.VirtualDom.node "s"


{-| Represents the title of a work. -}
cite : List (Attribute msg) -> List (Html msg) -> Html msg
cite =
  Elm.Kernel.VirtualDom.node "cite"


{-| Represents an inline quotation. -}
q : List (Attribute msg) -> List (Html msg) -> Html msg
q =
  Elm.Kernel.VirtualDom.node "q"


{-| Represents a term whose definition is contained in its nearest ancestor
content.
-}
dfn : List (Attribute msg) -> List (Html msg) -> Html msg
dfn =
  Elm.Kernel.VirtualDom.node "dfn"


{-| Represents an abbreviation or an acronym; the expansion of the
abbreviation can be represented in the title attribute.
-}
abbr : List (Attribute msg) -> List (Html msg) -> Html msg
abbr =
  Elm.Kernel.VirtualDom.node "abbr"


{-| Represents a date and time value; the machine-readable equivalent can be
represented in the datetime attribute.
-}
time : List (Attribute msg) -> List (Html msg) -> Html msg
time =
  Elm.Kernel.VirtualDom.node "time"


{-| Represents computer code. -}
code : List (Attribute msg) -> List (Html msg) -> Html msg
code =
  Elm.Kernel.VirtualDom.node "code"


{-| Represents a variable. Specific cases where it should be used include an
actual mathematical expression or programming context, an identifier
representing a constant, a symbol identifying a physical quantity, a function
parameter, or a mere placeholder in prose.
-}
var : List (Attribute msg) -> List (Html msg) -> Html msg
var =
  Elm.Kernel.VirtualDom.node "var"


{-| Represents the output of a program or a computer. -}
samp : List (Attribute msg) -> List (Html msg) -> Html msg
samp =
  Elm.Kernel.VirtualDom.node "samp"


{-| Represents user input, often from the keyboard, but not necessarily; it
may represent other input, like transcribed voice commands.
-}
kbd : List (Attribute msg) -> List (Html msg) -> Html msg
kbd =
  Elm.Kernel.VirtualDom.node "kbd"


{-| Represent a subscript. -}
sub : List (Attribute msg) -> List (Html msg) -> Html msg
sub =
  Elm.Kernel.VirtualDom.node "sub"


{-| Represent a superscript. -}
sup : List (Attribute msg) -> List (Html msg) -> Html msg
sup =
  Elm.Kernel.VirtualDom.node "sup"


{-| Represents some text in an alternate voice or mood, or at least of
different quality, such as a taxonomic designation, a technical term, an
idiomatic phrase, a thought, or a ship name.
-}
i : List (Attribute msg) -> List (Html msg) -> Html msg
i =
  Elm.Kernel.VirtualDom.node "i"


{-| Represents a text which to which attention is drawn for utilitarian
purposes. It doesn't convey extra importance and doesn't imply an alternate
voice.
-}
b : List (Attribute msg) -> List (Html msg) -> Html msg
b =
  Elm.Kernel.VirtualDom.node "b"


{-| Represents a non-textual annotation for which the conventional
presentation is underlining, such labeling the text as being misspelt or
labeling a proper name in Chinese text.
-}
u : List (Attribute msg) -> List (Html msg) -> Html msg
u =
  Elm.Kernel.VirtualDom.node "u"


{-| Represents text highlighted for reference purposes, that is for its
relevance in another context.
-}
mark : List (Attribute msg) -> List (Html msg) -> Html msg
mark =
  Elm.Kernel.VirtualDom.node "mark"


{-| Represents content to be marked with ruby annotations, short runs of text
presented alongside the text. This is often used in conjunction with East Asian
language where the annotations act as a guide for pronunciation, like the
Japanese furigana.
-}
ruby : List (Attribute msg) -> List (Html msg) -> Html msg
ruby =
  Elm.Kernel.VirtualDom.node "ruby"


{-| Represents the text of a ruby annotation. -}
rt : List (Attribute msg) -> List (Html msg) -> Html msg
rt =
  Elm.Kernel.VirtualDom.node "rt"


{-| Represents parenthesis around a ruby annotation, used to display the
annotation in an alternate way by browsers not supporting the standard display
for annotations.
-}
rp : List (Attribute msg) -> List (Html msg) -> Html msg
rp =
  Elm.Kernel.VirtualDom.node "rp"


{-| Represents text that must be isolated from its surrounding for
bidirectional text formatting. It allows embedding a span of text with a
different, or unknown, directionality.
-}
bdi : List (Attribute msg) -> List (Html msg) -> Html msg
bdi =
  Elm.Kernel.VirtualDom.node "bdi"


{-| Represents the directionality of its children, in order to explicitly
override the Unicode bidirectional algorithm.
-}
bdo : List (Attribute msg) -> List (Html msg) -> Html msg
bdo =
  Elm.Kernel.VirtualDom.node "bdo"


{-| Represents text with no specific meaning. This has to be used when no other
text-semantic element conveys an adequate meaning, which, in this case, is
often brought by global attributes like `class`, `lang`, or `dir`.
-}
span : List (Attribute msg) -> List (Html msg) -> Html msg
span =
  Elm.Kernel.VirtualDom.node "span"


{-| Represents a line break. -}
br : List (Attribute msg) -> List (Html msg) -> Html msg
br =
  Elm.Kernel.VirtualDom.node "br"


{-| Represents a line break opportunity, that is a suggested point for
wrapping text in order to improve readability of text split on several lines.
-}
wbr : List (Attribute msg) -> List (Html msg) -> Html msg
wbr =
  Elm.Kernel.VirtualDom.node "wbr"


-- EDITS

{-| Defines an addition to the document. -}
ins : List (Attribute msg) -> List (Html msg) -> Html msg
ins =
  Elm.Kernel.VirtualDom.node "ins"


{-| Defines a removal from the document. -}
del : List (Attribute msg) -> List (Html msg) -> Html msg
del =
  Elm.Kernel.VirtualDom.node "del"


-- EMBEDDED CONTENT

{-| Represents an image. -}
img : List (Attribute msg) -> List (Html msg) -> Html msg
img =
  Elm.Kernel.VirtualDom.node "img"


{-| Embedded an HTML document. -}
iframe : List (Attribute msg) -> List (Html msg) -> Html msg
iframe =
  Elm.Kernel.VirtualDom.node "iframe"


{-| Represents a integration point for an external, often non-HTML,
application or interactive content.
-}
embed : List (Attribute msg) -> List (Html msg) -> Html msg
embed =
  Elm.Kernel.VirtualDom.node "embed"


{-| Represents an external resource, which is treated as an image, an HTML
sub-document, or an external resource to be processed by a plug-in.
-}
object : List (Attribute msg) -> List (Html msg) -> Html msg
object =
  Elm.Kernel.VirtualDom.node "object"


{-| Defines parameters for use by plug-ins invoked by `object` elements. -}
param : List (Attribute msg) -> List (Html msg) -> Html msg
param =
  Elm.Kernel.VirtualDom.node "param"


{-| Represents a video, the associated audio and captions, and controls. -}
video : List (Attribute msg) -> List (Html msg) -> Html msg
video =
  Elm.Kernel.VirtualDom.node "video"


{-| Represents a sound or audio stream. -}
audio : List (Attribute msg) -> List (Html msg) -> Html msg
audio =
  Elm.Kernel.VirtualDom.node "audio"


{-| Allows authors to specify alternative media resources for media elements
like `video` or `audio`.
-}
source : List (Attribute msg) -> List (Html msg) -> Html msg
source =
  Elm.Kernel.VirtualDom.node "source"


{-| Allows authors to specify timed text track for media elements like `video`
or `audio`.
-}
track : List (Attribute msg) -> List (Html msg) -> Html msg
track =
  Elm.Kernel.VirtualDom.node "track"


{-| Represents a bitmap area for graphics rendering. -}
canvas : List (Attribute msg) -> List (Html msg) -> Html msg
canvas =
  Elm.Kernel.VirtualDom.node "canvas"


{-| Defines a mathematical formula. -}
math : List (Attribute msg) -> List (Html msg) -> Html msg
math =
  Elm.Kernel.VirtualDom.node "math"


-- TABULAR DATA

{-| Represents data with more than one dimension. -}
table : List (Attribute msg) -> List (Html msg) -> Html msg
table =
  Elm.Kernel.VirtualDom.node "table"


{-| Represents the title of a table. -}
caption : List (Attribute msg) -> List (Html msg) -> Html msg
caption =
  Elm.Kernel.VirtualDom.node "caption"


{-| Represents a set of one or more columns of a table. -}
colgroup : List (Attribute msg) -> List (Html msg) -> Html msg
colgroup =
  Elm.Kernel.VirtualDom.node "colgroup"


{-| Represents a column of a table. -}
col : List (Attribute msg) -> List (Html msg) -> Html msg
col =
  Elm.Kernel.VirtualDom.node "col"


{-| Represents the block of rows that describes the concrete data of a table.
-}
tbody : List (Attribute msg) -> List (Html msg) -> Html msg
tbody =
  Elm.Kernel.VirtualDom.node "tbody"


{-| Represents the block of rows that describes the column labels of a table.
-}
thead : List (Attribute msg) -> List (Html msg) -> Html msg
thead =
  Elm.Kernel.VirtualDom.node "thead"


{-| Represents the block of rows that describes the column summaries of a table.
-}
tfoot : List (Attribute msg) -> List (Html msg) -> Html msg
tfoot =
  Elm.Kernel.VirtualDom.node "tfoot"


{-| Represents a row of cells in a table. -}
tr : List (Attribute msg) -> List (Html msg) -> Html msg
tr =
  Elm.Kernel.VirtualDom.node "tr"


{-| Represents a data cell in a table. -}
td : List (Attribute msg) -> List (Html msg) -> Html msg
td =
  Elm.Kernel.VirtualDom.node "td"


{-| Represents a header cell in a table. -}
th : List (Attribute msg) -> List (Html msg) -> Html msg
th =
  Elm.Kernel.VirtualDom.node "th"


-- FORMS

{-| Represents a form, consisting of controls, that can be submitted to a
server for processing.
-}
form : List (Attribute msg) -> List (Html msg) -> Html msg
form =
  Elm.Kernel.VirtualDom.node "form"


{-| Represents a set of controls. -}
fieldset : List (Attribute msg) -> List (Html msg) -> Html msg
fieldset =
  Elm.Kernel.VirtualDom.node "fieldset"


{-| Represents the caption for a `fieldset`. -}
legend : List (Attribute msg) -> List (Html msg) -> Html msg
legend =
  Elm.Kernel.VirtualDom.node "legend"


{-| Represents the caption of a form control. -}
label : List (Attribute msg) -> List (Html msg) -> Html msg
label =
  Elm.Kernel.VirtualDom.node "label"


{-| Represents a typed data field allowing the user to edit the data. -}
input : List (Attribute msg) -> List (Html msg) -> Html msg
input =
  Elm.Kernel.VirtualDom.node "input"


{-| Represents a button. -}
button : List (Attribute msg) -> List (Html msg) -> Html msg
button =
  Elm.Kernel.VirtualDom.node "button"


{-| Represents a control allowing selection among a set of options. -}
select : List (Attribute msg) -> List (Html msg) -> Html msg
select =
  Elm.Kernel.VirtualDom.node "select"


{-| Represents a set of predefined options for other controls. -}
datalist : List (Attribute msg) -> List (Html msg) -> Html msg
datalist =
  Elm.Kernel.VirtualDom.node "datalist"


{-| Represents a set of options, logically grouped. -}
optgroup : List (Attribute msg) -> List (Html msg) -> Html msg
optgroup =
  Elm.Kernel.VirtualDom.node "optgroup"


{-| Represents an option in a `select` element or a suggestion of a `datalist`
element.
-}
option : List (Attribute msg) -> List (Html msg) -> Html msg
option =
  Elm.Kernel.VirtualDom.node "option"


{-| Represents a multiline text edit control. -}
textarea : List (Attribute msg) -> List (Html msg) -> Html msg
textarea =
  Elm.Kernel.VirtualDom.node "textarea"


{-| Represents the result of a calculation. -}
output : List (Attribute msg) -> List (Html msg) -> Html msg
output =
  Elm.Kernel.VirtualDom.node "output"


{-| Represents the completion progress of a task. -}
progress : List (Attribute msg) -> List (Html msg) -> Html msg
progress =
  Elm.Kernel.VirtualDom.node "progress"


{-| Represents a scalar measurement (or a fractional value), within a known
range.
-}
meter : List (Attribute msg) -> List (Html msg) -> Html msg
meter =
  Elm.Kernel.VirtualDom.node "meter"


-- INTERACTIVE ELEMENTS

{-| Represents a widget from which the user can obtain additional information
or controls.
-}
details : List (Attribute msg) -> List (Html msg) -> Html msg
details =
  Elm.Kernel.VirtualDom.node "details"


{-| Represents a summary, caption, or legend for a given `details`. -}
summary : List (Attribute msg) -> List (Html msg) -> Html msg
summary =
  Elm.Kernel.VirtualDom.node "summary"


{-| Represents a command that the user can invoke. -}
menuitem : List (Attribute msg) -> List (Html msg) -> Html msg
menuitem =
  Elm.Kernel.VirtualDom.node "menuitem"


{-| Represents a list of commands. -}
menu : List (Attribute msg) -> List (Html msg) -> Html msg
menu =
  Elm.Kernel.VirtualDom.node "menu"

