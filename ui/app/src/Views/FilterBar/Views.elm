module Views.FilterBar.Views exposing (view, viewCreatedByListBar, viewMatchersBar)

import Html exposing (Attribute, Html, a, button, div, i, input, small, span, text)
import Html.Attributes exposing (class, disabled, href, id, style, value)
import Html.Events exposing (keyCode, on, onClick, onInput)
import Utils.Filter exposing (Matcher, convertFilterMatcher)
import Utils.Keyboard exposing (keys, onKeyDown, onKeyUp)
import Utils.List
import Views.FilterBar.Types exposing (Model, Msg(..))
import Views.SilenceForm.Parsing exposing (newSilenceFromMatchers)


keys :
    { backspace : Int
    , enter : Int
    }
keys =
    { backspace = 8
    , enter = 13
    }


viewMatcher : Matcher -> Html Msg
viewMatcher matcher =
    div [ class "col col-auto" ]
        [ div [ class "btn-group mr-2 mb-2" ]
            [ button
                [ class "btn btn-outline-info"
                , onClick (DeleteFilterMatcher True matcher)
                ]
                [ text <| Utils.Filter.stringifyMatcher matcher
                ]
            , button
                [ class "btn btn-outline-danger"
                , onClick (DeleteFilterMatcher False matcher)
                ]
                [ text "×" ]
            ]
        ]


viewMatchers : List Matcher -> List (Html Msg)
viewMatchers matchers =
    matchers
        |> List.map viewMatcher


viewCreatedBy : String -> Html Msg
viewCreatedBy createdBy =
    div [ class "col col-auto" ]
        [ div [ class "btn-group mr-2 mb-2" ]
            [ button
                [ class "btn btn-outline-info"
                , onClick (DeleteFilterCreatedBy True createdBy)
                ]
                [ text <| createdBy
                ]
            , button
                [ class "btn btn-outline-danger"
                , onClick (DeleteFilterCreatedBy False createdBy)
                ]
                [ text "×" ]
            ]
        ]


viewCreatedByList : List String -> List (Html Msg)
viewCreatedByList createdByList =
    createdByList
        |> List.map viewCreatedBy


view : { showSilenceButton : Bool } -> Model -> Html Msg
view showSilenceButton model =
    div []
        [ viewMatchersBar showSilenceButton True model
        , if model.showCreatedByBar || not (List.isEmpty model.createdByList) then
            viewCreatedByListBar model

          else
            div [] []
        ]


viewMatchersBar : { showSilenceButton : Bool } -> Bool -> Model -> Html Msg
viewMatchersBar { showSilenceButton } showCreatedByButton { matchers, createdByList, matcherText, createdByText, backspacePressed } =
    let
        maybeMatcher =
            Utils.Filter.parseMatcher matcherText

        maybeLastMatcher =
            Utils.List.lastElem matchers

        className =
            if matcherText == "" then
                ""

            else
                case maybeMatcher of
                    Just _ ->
                        "has-success"

                    Nothing ->
                        "has-danger"

        keyDown key =
            if key == keys.enter then
                maybeMatcher
                    |> Maybe.map (AddFilterMatcher True)
                    |> Maybe.withDefault Noop

            else if key == keys.backspace then
                if matcherText == "" then
                    case ( backspacePressed, maybeLastMatcher ) of
                        ( False, Just lastMatcher ) ->
                            DeleteFilterMatcher True lastMatcher

                        _ ->
                            Noop

                else
                    PressingBackspace True

            else
                Noop

        keyUp key =
            if key == keys.backspace then
                PressingBackspace False

            else
                Noop

        onClickAttr =
            maybeMatcher
                |> Maybe.map (AddFilterMatcher True)
                |> Maybe.withDefault Noop
                |> onClick

        isDisabled =
            maybeMatcher == Nothing

        dataMatchers =
            matchers
                |> List.map convertFilterMatcher
    in
    div [ class "row no-gutters align-items-start" ]
        (viewMatchers matchers
            ++ [ div
                    [ class ("col " ++ className)
                    , style "min-width"
                        (if showSilenceButton then
                            "300px"

                         else
                            "200px"
                        )
                    ]
                    [ div [ class "row no-gutters align-content-stretch" ]
                        [ div [ class "col input-group" ]
                            [ input
                                [ id "filter-bar-matcher"
                                , class "form-control"
                                , value matcherText
                                , onKeyDown keyDown
                                , onKeyUp keyUp
                                , onInput UpdateMatcherText
                                ]
                                []
                            , span
                                [ class "input-group-btn" ]
                                [ button [ class "btn btn-primary", disabled isDisabled, onClickAttr ] [ text "+" ] ]
                            ]
                        , if showSilenceButton then
                            div [ class "col col-auto input-group-btn ml-2" ]
                                [ div [ class "input-group" ]
                                    [ a
                                        [ class "btn btn-outline-info"
                                        , href (newSilenceFromMatchers dataMatchers)
                                        ]
                                        [ i [ class "fa fa-bell-slash-o mr-2" ] []
                                        , text "Silence"
                                        ]
                                    ]
                                ]

                          else
                            text ""
                        ]
                    , small [ class "btn-toolbar form-text text-muted" ]
                        [ span [ class "d-flex align-items-center" ] [ text "Custom matcher, e.g." ]
                        , button
                            [ class "btn btn-link btn-sm align-baseline"
                            , onClick (UpdateMatcherText exampleMatcher)
                            ]
                            [ text exampleMatcher ]
                        , if showCreatedByButton then
                            div [ class "btn-group ml-auto" ]
                                [ button [ class "btn btn-sm btn-outline-secondary", onClick ShowCreatedByBar, disabled <| not (List.isEmpty createdByList) ] [ text "created by" ] ]

                          else
                            div [] []
                        ]
                    ]
               ]
        )


viewCreatedByListBar : Model -> Html Msg
viewCreatedByListBar { matchers, createdByList, matcherText, createdByText, backspacePressed } =
    let
        maybeCreatedBy =
            Utils.Filter.parseCreatedBy createdByText

        maybeLastCreatedBy =
            Utils.List.lastElem createdByList

        className =
            if createdByText == "" then
                ""

            else
                case maybeCreatedBy of
                    Just _ ->
                        "has-success"

                    Nothing ->
                        "has-danger"

        keyDown key =
            if key == keys.enter then
                maybeCreatedBy
                    |> Maybe.map (AddFilterCreatedBy True)
                    |> Maybe.withDefault Noop

            else if key == keys.backspace then
                if createdByText == "" then
                    case ( backspacePressed, maybeLastCreatedBy ) of
                        ( False, Just lastCreatedBy ) ->
                            DeleteFilterCreatedBy True lastCreatedBy

                        _ ->
                            Noop

                else
                    PressingBackspace True

            else
                Noop

        keyUp key =
            if key == keys.backspace then
                PressingBackspace False

            else
                Noop

        isDisabled =
            maybeCreatedBy == Nothing

        onClickAttr =
            maybeCreatedBy
                |> Maybe.map (AddFilterCreatedBy True)
                |> Maybe.withDefault Noop
                |> onClick
    in
    div [ class "row no-gutters align-items-start mt-1" ]
        (viewCreatedByList createdByList
            ++ [ div
                    [ class ("col " ++ className) ]
                    [ div [ class "row no-gutters align-content-stretch" ]
                        [ div [ class "col input-group" ]
                            [ input
                                [ id "filter-bar-created-by"
                                , class "form-control"
                                , value createdByText
                                , onKeyDown keyDown
                                , onKeyUp keyUp
                                , onInput UpdateCreatedByText
                                ]
                                []
                            , span
                                [ class "input-group-btn" ]
                                [ button [ class "btn btn-primary", disabled isDisabled, onClickAttr ] [ text "+" ] ]
                            ]
                        ]
                    , small [ class "btn-toolbar form-text text-muted" ]
                        [ span [ class "d-flex align-items-center" ] [ text "Created By ..." ] ]
                    ]
               ]
        )


exampleMatcher : String
exampleMatcher =
    "env=\"production\""
