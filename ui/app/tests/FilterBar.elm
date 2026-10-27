module FilterBar exposing (update)

import Expect
import Test exposing (..)
import Utils.Filter
import Views.FilterBar.Types as FilterBarTypes
import Views.FilterBar.Updates as FilterBarUpdates


matcher : String -> String -> Utils.Filter.Matcher
matcher key value =
    { key = key, op = Utils.Filter.Eq, value = value }


model : List Utils.Filter.Matcher -> FilterBarTypes.Model
model matchers =
    FilterBarTypes.initFilterBar matchers


update : Test
update =
    describe "FilterBar update"
        [ test "pins and unpins a matcher" <|
            \() ->
                let
                    first =
                        matcher "alertname" "ExampleAlert"

                    initial =
                        model [ first ]

                    ( pinned, _, _ ) =
                        FilterBarUpdates.update (FilterBarTypes.KeepFilterMatcher first) initial

                    ( unpinned, _, _ ) =
                        FilterBarUpdates.update (FilterBarTypes.KeepFilterMatcher first) pinned
                in
                Expect.equal [] unpinned.keepMatchers
        , test "deleting a pinned matcher removes its pin" <|
            \() ->
                let
                    first =
                        matcher "alertname" "ExampleAlert"

                    base =
                        model [ first ]

                    initial =
                        { base | keepMatchers = [ first ] }

                    ( updated, _, _ ) =
                        FilterBarUpdates.update (FilterBarTypes.DeleteFilterMatcher False first) initial
                in
                Expect.equal [] updated.keepMatchers
        , test "setMatchers clears pins from the previous filter" <|
            \() ->
                let
                    first =
                        matcher "alertname" "ExampleAlert"

                    base =
                        model [ first ]

                    initial =
                        { base | keepMatchers = [ first ] }

                    filterBase =
                        Utils.Filter.nullFilter

                    filter =
                        { filterBase | text = Just (Utils.Filter.stringifyFilter [ first ]) }

                    updated =
                        FilterBarUpdates.setMatchers filter initial
                in
                Expect.equal [] updated.keepMatchers
        , test "transfers a pin to the edited matcher" <|
            \() ->
                let
                    first =
                        matcher "alertname" "ExampleAlert"

                    edited =
                        matcher "alertname" "OtherAlert"

                    base =
                        model [ first ]

                    initial =
                        { base | keepMatchers = [ first ] }

                    ( editing, _, _ ) =
                        FilterBarUpdates.update (FilterBarTypes.DeleteFilterMatcher True first) initial

                    ( updated, _, _ ) =
                        FilterBarUpdates.update (FilterBarTypes.AddFilterMatcher True edited) editing
                in
                Expect.equal [ edited ] updated.keepMatchers
        ]
